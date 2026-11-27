package uitest

import "testing"

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

	issues, err := templateSourceIssues(path, source, cfg, "error")
	if err != nil {
		t.Fatal(err)
	}

	if len(issues) != 2 {
		t.Fatalf("expected two issues, got %v", issues)
	}

	assertHasIssueSubstring(t, issues, `example.templ:6:7: error: inline style "height: 12px"`)
	assertHasIssueSubstring(t, issues, `example.templ:8:10: error: cross-control/global trigger`)
}

func TestStylesheetIssues(t *testing.T) {
	source := `/* div { width: 42px; } */
@media (max-width: 700px) {
	div { --column-width: 40px; width: var(--column-width); min-height:
		12px; }
}`

	issues := stylesheetIssues("example.css", source, "warning")
	if len(issues) != 1 {
		t.Fatalf("expected one issue, got %v", issues)
	}

	assertHasIssueSubstring(t, issues, `example.css:3:58: warning: hardcoded pixel dimension`)
}
