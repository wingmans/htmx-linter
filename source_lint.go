package uitest

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	templparser "github.com/a-h/templ/parser/v2"
	"github.com/a-h/templ/parser/v2/visitor"
)

var (
	cssCommentRe  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssFixedDimRe = regexp.MustCompile(`(?i)[{;]\s*((?:min-|max-)?(?:width|height)\s*:\s*[+-]?(?:\d+\.?\d*|\.\d+)px\b)`)
)

// AssertSourceRules checks authored templates, static HTML, and stylesheets under rootDir.
func AssertSourceRules(t *testing.T, rootDir string, cfg LintConfig) {
	t.Helper()
	ReportSourceRules(t, rootDir, cfg, true)
}

// ReportSourceRules emits source findings as warnings, or as failing errors
// when warningsAsErrors is true. Scan failures always fail the test.
func ReportSourceRules(t *testing.T, rootDir string, cfg LintConfig, warningsAsErrors bool) {
	t.Helper()

	level := "warning"
	if warningsAsErrors {
		level = "error"
	}

	absoluteRoot, err := filepath.Abs(rootDir)
	if err != nil {
		t.Fatalf("failed resolving %q: %v", rootDir, err)
	}

	root, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		t.Fatalf("failed opening %q: %v", rootDir, err)
	}

	defer func() {
		if err := root.Close(); err != nil {
			t.Errorf("failed closing %q: %v", rootDir, err)
		}
	}()

	var findings []string

	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".templ" && ext != ".css" && ext != ".html" {
			return nil
		}

		data, err := root.ReadFile(path)
		if err != nil {
			return err
		}

		sourcePath := filepath.Join(absoluteRoot, filepath.FromSlash(path))

		switch ext {
		case ".templ":
			issues, err := templateSourceIssues(sourcePath, string(data), cfg, level)
			if err != nil {
				return err
			}

			findings = append(findings, issues...)
		case ".css":
			findings = append(findings, stylesheetIssues(sourcePath, string(data), level)...)
		case ".html":
			page := ParsePage(t, http.StatusOK, nil, string(data))
			for _, issue := range page.CollectLintIssues(cfg) {
				findings = append(findings, fmt.Sprintf("%s:1:1: %s: %s", sourcePath, level, issue))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed walking %q: %v", rootDir, err)
	}

	if len(findings) > 0 {
		// Emit diagnostics without a testing.T prefix so editors can link each line.
		fmt.Fprintf(os.Stderr, "UI source violations:\n%s\n", strings.Join(findings, "\n"))
	}

	if warningsAsErrors && len(findings) > 0 {
		t.FailNow()
	}
}

func templateSourceIssues(path, source string, cfg LintConfig, level string) ([]string, error) {
	templateFile, err := templparser.ParseString(source)
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", path, err)
	}

	var (
		findings  []string
		elementID string
	)

	v := visitor.New()
	visitElement := v.Element

	v.Element = func(element *templparser.Element) error {
		parentID := elementID
		elementID = ""

		for _, attr := range element.Attributes {
			if attr, ok := attr.(*templparser.ConstantAttribute); ok && strings.EqualFold(attr.Key.String(), "id") {
				elementID = strings.TrimSpace(attr.Value)
			}
		}

		err := visitElement(element)
		elementID = parentID

		return err
	}
	v.ConstantAttribute = func(attr *templparser.ConstantAttribute) error {
		pos := attr.Range.From
		switch strings.ToLower(attr.Key.String()) {
		case "style":
			if !usesCSSVariables(attr.Value) {
				findings = append(findings, fmt.Sprintf("%s:%d:%d: %s: inline style %q (prefer a CSS class or custom property)",
					path, pos.Line+1, pos.Col+1, level, attr.Value))
			}
		case "hx-trigger", "data-hx-trigger":
			if cfg.StrictScopedTriggers && (elementID == "" || !cfg.AllowedTriggerIDs[elementID]) && usesExternalTrigger(attr.Value) {
				findings = append(findings, fmt.Sprintf("%s:%d:%d: %s: cross-control/global trigger %q", path, pos.Line+1, pos.Col+1, level, attr.Value))
			}
		}

		return nil
	}
	v.ExpressionAttribute = func(attr *templparser.ExpressionAttribute) error {
		if strings.EqualFold(attr.Key.String(), "style") && !usesCSSVariables(styleExpressionFormat(attr.Expression.Value)) {
			pos := attr.Expression.Range.From
			findings = append(findings, fmt.Sprintf("%s:%d:%d: %s: inline style expression %q must set or use CSS custom properties",
				path, pos.Line+1, pos.Col+1, level, attr.Expression.Value))
		}

		return nil
	}

	err = v.VisitTemplateFile(templateFile)

	return findings, err
}

func usesCSSVariables(style string) bool {
	for declaration := range strings.SplitSeq(style, ";") {
		property, value, ok := strings.Cut(strings.TrimSpace(declaration), ":")
		if ok && !strings.HasPrefix(property, "--") && !strings.Contains(strings.ToLower(value), "var(--") {
			return false
		}
	}

	return style != ""
}

// styleExpressionFormat reads the literal CSS in templ style expressions that
// use fmt.Sprintf to insert data-dependent values.
func styleExpressionFormat(source string) string {
	expr, err := goparser.ParseExpr(source)
	if err != nil {
		return ""
	}

	if call, ok := expr.(*ast.CallExpr); ok && len(call.Args) > 0 {
		fn, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || fn.Sel.Name != "Sprintf" {
			return ""
		}

		pkg, ok := fn.X.(*ast.Ident)
		if !ok || pkg.Name != "fmt" {
			return ""
		}

		expr = call.Args[0]
	}

	if literal, ok := expr.(*ast.BasicLit); ok {
		value, _ := strconv.Unquote(literal.Value)

		return value
	}

	return ""
}

func stylesheetIssues(path, source, level string) []string {
	// Mask comments while keeping offsets and line numbers intact.
	css := cssCommentRe.ReplaceAllStringFunc(source, func(comment string) string {
		masked := []byte(comment)
		for i, b := range masked {
			if b != '\n' {
				masked[i] = ' '
			}
		}

		return string(masked)
	})

	matches := cssFixedDimRe.FindAllStringSubmatchIndex(css, -1)
	findings := make([]string, 0, len(matches))

	for _, match := range matches {
		start := match[2]
		line := strings.Count(css[:start], "\n") + 1
		column := start - strings.LastIndexByte(css[:start], '\n')
		findings = append(findings, fmt.Sprintf("%s:%d:%d: %s: hardcoded pixel dimension %q (prefer var(--...), rem, or %%)",
			path, line, column, level, css[start:match[3]]))
	}

	return findings
}
