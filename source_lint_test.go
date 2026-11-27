package uitest

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTemplateSourceIssues(t *testing.T) {
	source := `package test

templ Example() {
	<div style={ fmt.Sprintf("--periods: %d", 3) }/>
	<div style='width: var(--column-width)'/>
	<div style="height: 12px"/>
	<button id="allowed" hx-trigger="click from:body"/>
	<button data-hx-trigger="click
		from:body"/>
}
`
	path := "example.templ"
	cfg := DefaultLintConfig()
	cfg.AllowedTriggerIDs = map[string]bool{"allowed": true}

	issues, err := templateSourceIssues(path, source, cfg)
	if err != nil {
		t.Fatal(err)
	}

	want := []SourceDiagnostic{
		{Path: path, Line: 6, Column: 7, Message: `inline style "height: 12px" (prefer a CSS class or custom property)`},
		{Path: path, Line: 8, Column: 10, Message: `cross-control/global trigger "click\n\t\tfrom:body"`},
	}
	if !slices.Equal(issues, want) {
		t.Fatalf("got %+v, want %+v", issues, want)
	}
}

func TestStylesheetIssues(t *testing.T) {
	source := `/* div { width: 42px; } */
@media (max-width: 700px) {
	div { --column-width: 40px; width: var(--column-width); min-height:
		12px; }
}`

	issues := stylesheetIssues("example.css", source)
	want := []SourceDiagnostic{{
		Path:    "example.css",
		Line:    3,
		Column:  58,
		Message: `hardcoded pixel dimension "min-height:\n\t\t12px" (prefer var(--...), rem, or %)`,
	}}
	if !slices.Equal(issues, want) {
		t.Fatalf("got %+v, want %+v", issues, want)
	}
}

func TestScanSourceRules(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string]string{
		"index.html": `<div id="item"></div><div id="item"></div>`,
		"notes.txt":  `<div id=""></div>`,
		"site.css":   `div { width: 12px; }`,
		"view.templ": "package example\n\ntempl View() {\n\t<div style=\"height: 12px\"/>\n}\n",
	}
	for name, contents := range fixtures {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cfg := DefaultLintConfig()
	issues, err := ScanSourceRules(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []SourceDiagnostic{
		{Path: filepath.Join(root, "index.html"), Line: 1, Column: 1, Message: `duplicate id="item"`},
		{Path: filepath.Join(root, "site.css"), Line: 1, Column: 7, Message: `hardcoded pixel dimension "width: 12px" (prefer var(--...), rem, or %)`},
		{Path: filepath.Join(root, "view.templ"), Line: 4, Column: 7, Message: `inline style "height: 12px" (prefer a CSS class or custom property)`},
	}
	if !slices.Equal(issues, want) {
		t.Fatalf("got %+v, want %+v", issues, want)
	}

	path := filepath.Join(root, "view.templ")
	if err := os.WriteFile(path, []byte("package example\n\ntempl View() {\n\t<div>\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	issues, err = ScanSourceRules(root, cfg)
	if err == nil || !strings.Contains(err.Error(), path) || len(issues) != 0 {
		t.Fatalf("got %+v, %v, want a scan error for %s", issues, err, path)
	}
}
