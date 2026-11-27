package uitest

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	templparser "github.com/a-h/templ/parser/v2"
	"github.com/a-h/templ/parser/v2/visitor"
)

var (
	cssCommentRe  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssFixedDimRe = regexp.MustCompile(`(?i)[{;]\s*((?:min-|max-)?(?:width|height)\s*:\s*[+-]?(?:\d+\.?\d*|\.\d+)px\b)`)
)

// SourceDiagnostic describes a lint finding with an absolute file path and
// one-based coordinates. HTML findings currently use line 1, column 1.
type SourceDiagnostic struct {
	Path    string
	Line    int
	Column  int
	Message string
}

// AssertSourceRules checks authored templates, static HTML, and stylesheets under rootDir.
func AssertSourceRules(t *testing.T, rootDir string, cfg LintConfig) {
	t.Helper()
	ReportSourceRules(t, rootDir, cfg, true)
}

// ReportSourceRules emits source findings as warnings, or as failing errors
// when warningsAsErrors is true. Scan failures always fail the test.
func ReportSourceRules(t *testing.T, rootDir string, cfg LintConfig, warningsAsErrors bool) {
	t.Helper()

	findings, err := ScanSourceRules(rootDir, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(findings) == 0 {
		return
	}

	level := "warning"
	if warningsAsErrors {
		level = "error"
	}

	// Emit diagnostics without a testing.T prefix so editors can link each line.
	var output strings.Builder
	output.WriteString("UI source violations:\n")
	for _, finding := range findings {
		fmt.Fprintf(&output, "%s:%d:%d: %s: %s\n", finding.Path, finding.Line, finding.Column, level, finding.Message)
	}
	fmt.Fprint(os.Stderr, output.String())

	if warningsAsErrors {
		t.FailNow()
	}
}

// ScanSourceRules checks .templ, .css, and .html files under rootDir.
// It returns diagnostics on a successful scan and an error if scanning fails.
func ScanSourceRules(rootDir string, cfg LintConfig) ([]SourceDiagnostic, error) {
	absoluteRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed resolving %q: %w", rootDir, err)
	}

	root, err := os.OpenRoot(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("failed opening %q: %w", rootDir, err)
	}

	var findings []SourceDiagnostic

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
			issues, err := templateSourceIssues(sourcePath, string(data), cfg)
			if err != nil {
				return err
			}

			findings = append(findings, issues...)
		case ".css":
			findings = append(findings, stylesheetIssues(sourcePath, string(data))...)
		case ".html":
			body := string(data)
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
			if err != nil {
				return fmt.Errorf("parse HTML %s: %w", sourcePath, err)
			}
			for _, issue := range collectLintIssues(doc, body, cfg) {
				findings = append(findings, SourceDiagnostic{Path: sourcePath, Line: 1, Column: 1, Message: issue})
			}
		}

		return nil
	})
	closeErr := root.Close()
	if err != nil {
		return nil, fmt.Errorf("failed walking %q: %w", rootDir, err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("failed closing %q: %w", rootDir, closeErr)
	}

	return findings, nil
}

func templateSourceIssues(path, source string, cfg LintConfig) ([]SourceDiagnostic, error) {
	templateFile, err := templparser.ParseString(source)
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", path, err)
	}

	var (
		findings  []SourceDiagnostic
		elementID string
	)
	addFinding := func(pos templparser.Position, message string) {
		findings = append(findings, SourceDiagnostic{Path: path, Line: int(pos.Line) + 1, Column: int(pos.Col) + 1, Message: message})
	}

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
				addFinding(pos, fmt.Sprintf("inline style %q (prefer a CSS class or custom property)", attr.Value))
			}
		case "hx-trigger", "data-hx-trigger":
			if cfg.StrictScopedTriggers && (elementID == "" || !cfg.AllowedTriggerIDs[elementID]) && usesExternalTrigger(attr.Value) {
				addFinding(pos, fmt.Sprintf("cross-control/global trigger %q", attr.Value))
			}
		}

		return nil
	}
	v.ExpressionAttribute = func(attr *templparser.ExpressionAttribute) error {
		if strings.EqualFold(attr.Key.String(), "style") && !usesCSSVariables(styleExpressionFormat(attr.Expression.Value)) {
			pos := attr.Expression.Range.From
			addFinding(pos, fmt.Sprintf("inline style expression %q must set or use CSS custom properties", attr.Expression.Value))
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

func stylesheetIssues(path, source string) []SourceDiagnostic {
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
	findings := make([]SourceDiagnostic, 0, len(matches))

	for _, match := range matches {
		start := match[2]
		line := strings.Count(css[:start], "\n") + 1
		column := start - strings.LastIndexByte(css[:start], '\n')
		findings = append(findings, SourceDiagnostic{
			Path:    path,
			Line:    line,
			Column:  column,
			Message: fmt.Sprintf("hardcoded pixel dimension %q (prefer var(--...), rem, or %%)", css[start:match[3]]),
		})
	}

	return findings
}
