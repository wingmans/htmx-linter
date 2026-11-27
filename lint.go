package uitest

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

var externalTriggerRe = regexp.MustCompile(`(?i)(?:^|[[:space:],])from\s*:`)

func usesExternalTrigger(value string) bool {
	return externalTriggerRe.MatchString(value)
}

// LintConfig defines project-specific structural, trigger, and style rules.
type LintConfig struct {
	KnownTargets         map[string]bool
	BanInlineStyles      bool
	BannedStyleProps     map[string]bool
	StrictScopedTriggers bool
	AllowedTriggerIDs    map[string]bool
}

// DefaultLintConfig returns the baseline HTML and HTMX contracts.
func DefaultLintConfig() LintConfig {
	return LintConfig{
		BannedStyleProps: map[string]bool{
			"width":      true,
			"height":     true,
			"min-width":  true,
			"max-width":  true,
			"min-height": true,
			"max-height": true,
		},
		StrictScopedTriggers: true,
	}
}

// CollectLintIssues evaluates all HTML lint rules and returns human-readable issues.
func (p *Page) CollectLintIssues(cfg LintConfig) []string {
	return collectLintIssues(p.Doc, p.Body, cfg)
}

func collectLintIssues(doc *goquery.Document, body string, cfg LintConfig) []string {
	var issues []string

	seenIDs := make(map[string]int)

	// 1. Enforce unique, non-empty element IDs.
	doc.Find("[id]").Each(func(_ int, s *goquery.Selection) {
		id := strings.TrimSpace(s.AttrOr("id", ""))
		if id == "" {
			issues = append(issues, fmt.Sprintf("empty id on <%s>", goquery.NodeName(s)))

			return
		}

		seenIDs[id]++
		if seenIDs[id] == 2 {
			issues = append(issues, fmt.Sprintf("duplicate id=%q", id))
		}
	})

	// 2. Check static #id hx-target attributes against KnownTargets and full-page DOM.
	isFullPage, firstTag := documentStart(body)
	doc.Find("[hx-target]").Each(func(_ int, s *goquery.Selection) {
		target := strings.TrimSpace(s.AttrOr("hx-target", ""))
		if !strings.HasPrefix(target, "#") || strings.ContainsAny(target, " >+~.[]:,") {
			return
		}

		targetID := strings.TrimPrefix(target, "#")

		if len(cfg.KnownTargets) > 0 && !cfg.KnownTargets[targetID] {
			issues = append(issues, fmt.Sprintf("<%s> targets unknown id %q (not in KnownTargets)", goquery.NodeName(s), target))
		}

		if isFullPage && seenIDs[targetID] == 0 {
			issues = append(issues, fmt.Sprintf("<%s> targets %q, which is missing from full page", goquery.NodeName(s), target))
		}
	})

	// 3. Check hx-swap-oob targets, nesting in partial fragments, and bare table nodes.
	if !isFullPage {
		switch firstTag {
		case "tr", "td", "th", "tbody", "thead", "tfoot":
			issues = append(issues, fmt.Sprintf("partial fragment starts with bare <%s> outside <table>", firstTag))
		}
	}

	doc.Find("[hx-swap-oob]").Each(func(_ int, s *goquery.Selection) {
		oob := strings.TrimSpace(s.AttrOr("hx-swap-oob", ""))
		targetID := strings.TrimSpace(s.AttrOr("id", ""))

		if _, id, ok := strings.Cut(oob, "#"); ok {
			targetID = strings.TrimSpace(id)
		}

		if targetID == "" {
			issues = append(issues, fmt.Sprintf("<%s> has hx-swap-oob=%q without a resolvable target id", goquery.NodeName(s), oob))
		} else if len(cfg.KnownTargets) > 0 && !cfg.KnownTargets[targetID] {
			issues = append(issues, fmt.Sprintf("<%s> OOB-targets unknown id #%s", goquery.NodeName(s), targetID))
		}

		if !isFullPage && !s.Parent().Is("body") {
			issues = append(issues, fmt.Sprintf("OOB element #%s must be top-level in partial fragment, not nested inside <%s>",
				targetID, goquery.NodeName(s.Parent())))
		}
	})

	doc.Find("tr[hx-swap-oob], td[hx-swap-oob], th[hx-swap-oob], tbody[hx-swap-oob]").Each(func(_ int, s *goquery.Selection) {
		if s.Closest("table").Length() == 0 {
			issues = append(issues, fmt.Sprintf("<%s id=%q> uses hx-swap-oob without an enclosing <table> wrapper",
				goquery.NodeName(s), s.AttrOr("id", "(no id)")))
		}
	})

	// 4. Require explicit type="button" on <button hx-*> inside <form>.
	const htmxFormButtons = "form button[hx-get], form button[hx-post], form button[hx-put], form button[hx-patch], form button[hx-delete]"
	doc.Find(htmxFormButtons).Each(func(_ int, s *goquery.Selection) {
		if strings.ToLower(strings.TrimSpace(s.AttrOr("type", ""))) != "button" {
			issues = append(issues, fmt.Sprintf("<button id=%q> inside <form> uses hx-* without type=\"button\"", s.AttrOr("id", "(no id)")))
		}
	})

	// 5. Enforce project CSS / inline-style rules.
	doc.Find("[style]").Each(func(_ int, s *goquery.Selection) {
		raw := strings.TrimSpace(s.AttrOr("style", ""))
		if raw == "" {
			return
		}

		if cfg.BanInlineStyles {
			issues = append(issues, fmt.Sprintf("<%s id=%q> uses forbidden inline style=%q", goquery.NodeName(s), s.AttrOr("id", "(no id)"), raw))

			return
		}

		for decl := range strings.SplitSeq(strings.ToLower(raw), ";") {
			key, value, ok := strings.Cut(decl, ":")
			if !ok {
				continue
			}

			prop := strings.TrimSpace(key)

			val := strings.TrimSpace(value)
			if cfg.BannedStyleProps[prop] && !strings.Contains(val, "var(--") {
				issues = append(issues, fmt.Sprintf("<%s id=%q> has hardcoded %s: %s (use CSS class or var(--...))",
					goquery.NodeName(s), s.AttrOr("id", "(no id)"), prop, val))
			}
		}
	})

	// 6. Enforce scoped triggers unless explicitly allowlisted by element ID.
	if cfg.StrictScopedTriggers {
		doc.Find("[hx-trigger], [data-hx-trigger]").Each(func(_ int, s *goquery.Selection) {
			raw := strings.TrimSpace(s.AttrOr("hx-trigger", s.AttrOr("data-hx-trigger", "")))

			id := strings.TrimSpace(s.AttrOr("id", ""))
			if id != "" && cfg.AllowedTriggerIDs[id] {
				return
			}

			if usesExternalTrigger(raw) {
				issues = append(issues, fmt.Sprintf(
					"<%s id=%q> uses cross-control/global trigger hx-trigger=%q (add id to AllowedTriggerIDs if intentional)",
					goquery.NodeName(s), id, raw,
				))
			}
		})
	}

	return issues
}

// documentStart examines the source because the DOM parser inserts an <html>
// element even when the response is a fragment.
func documentStart(body string) (bool, string) {
	tokens := html.NewTokenizer(strings.NewReader(body))
	for {
		switch tokens.Next() {
		case html.ErrorToken:
			return false, ""
		case html.DoctypeToken:
			return true, ""
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := tokens.TagName()
			tag := string(name)

			return tag == "html", tag
		case html.TextToken, html.EndTagToken, html.CommentToken:
			continue
		}
	}
}

// Lint fails the test if CollectLintIssues reports any contract violations.
func (p *Page) Lint(cfg LintConfig) *Page {
	p.t.Helper()

	if issues := p.CollectLintIssues(cfg); len(issues) > 0 {
		p.t.Fatalf("HTML lint failed:\n  - %s", strings.Join(issues, "\n  - "))
	}

	return p
}
