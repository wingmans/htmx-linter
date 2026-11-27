package uitest

import (
	"net/http"
	"strings"
	"testing"
)

func assertHasIssueSubstring(t *testing.T, issues []string, wantSubstr string) {
	t.Helper()

	joined := strings.Join(issues, "\n")
	if !strings.Contains(joined, wantSubstr) {
		t.Fatalf("expected issue containing %q, got:\n%s", wantSubstr, joined)
	}
}

func TestCollectLintIssues_CatchesDuplicateIDsAndHardcodedWidths(t *testing.T) {
	html := `<!doctype html>
<html>
<body>
  <div id="tuple-list" style="width: 320px"></div>
  <div id="tuple-list" style="height: var(--row-height)"></div>
</body>
</html>`

	p := ParsePage(t, http.StatusOK, nil, html)
	cfg := DefaultLintConfig()

	issues := p.CollectLintIssues(cfg)
	assertHasIssueSubstring(t, issues, `duplicate id="tuple-list"`)
	assertHasIssueSubstring(t, issues, `has hardcoded width: 320px`)

	if strings.Contains(strings.Join(issues, "\n"), "row-height") {
		t.Fatalf("unexpected false positive for CSS variable: %v", issues)
	}
}

func TestCollectLintIssues_CatchesBareOOBTableRow(t *testing.T) {
	fragment := `<tr id="row-1" hx-swap-oob="outerHTML"><td>doc-1</td></tr>`

	p := ParsePage(t, http.StatusOK, nil, fragment)
	cfg := DefaultLintConfig()

	issues := p.CollectLintIssues(cfg)
	assertHasIssueSubstring(t, issues, "bare <tr")
}

func TestCollectLintIssues_StrictScopedTriggersAndAllowlist(t *testing.T) {
	html := `<!doctype html>
<html>
<body>
  <div id="tuple-list"></div>
  <select id="type-filter" hx-get="/tuples" hx-target="#tuple-list" hx-trigger="change, input from:#object-search"></select>
  <select id="global-filter" hx-get="/tuples" hx-target="#tuple-list" hx-trigger="change from:body"></select>
</body>
</html>`

	p := ParsePage(t, http.StatusOK, nil, html)
	cfg := DefaultLintConfig()
	cfg.AllowedTriggerIDs = map[string]bool{
		"type-filter": true,
	}

	issues := p.CollectLintIssues(cfg)
	assertHasIssueSubstring(t, issues, `id="global-filter"`)

	if strings.Contains(strings.Join(issues, "\n"), `id="type-filter"`) {
		t.Fatalf("allowlisted control should not fail: %v", issues)
	}
}

func TestCollectLintIssuesDistinguishesDocumentsAndFragments(t *testing.T) {
	const markup = `<button hx-get="/" hx-target="#workspace">Reload</button>`

	cfg := DefaultLintConfig()

	fragment := ParsePage(t, http.StatusOK, nil, markup)
	if issues := fragment.CollectLintIssues(cfg); len(issues) != 0 {
		t.Fatalf("unexpected fragment issues: %v", issues)
	}

	page := ParsePage(t, http.StatusOK, nil, "<!-- page -->\n<!doctype html>"+markup)
	assertHasIssueSubstring(t, page.CollectLintIssues(cfg), `targets "#workspace", which is missing from full page`)
}
