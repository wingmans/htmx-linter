package uitest

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ExtractOOBTargetIDs returns the target IDs referenced by [hx-swap-oob] nodes.
func (p *Page) ExtractOOBTargetIDs() []string {
	p.t.Helper()

	var ids []string

	p.Doc.Find("[hx-swap-oob]").Each(func(_ int, s *goquery.Selection) {
		oob := strings.TrimSpace(s.AttrOr("hx-swap-oob", ""))
		targetID := strings.TrimSpace(s.AttrOr("id", ""))

		if _, id, ok := strings.Cut(oob, "#"); ok {
			targetID = strings.TrimSpace(id)
		}

		if targetID != "" {
			ids = append(ids, targetID)
		}
	})

	return ids
}

// AssertOOBTargetsExistOnFullPage verifies that every OOB target on a partial
// fragment is present on the reference full page.
func (p *Page) AssertOOBTargetsExistOnFullPage(fullPage *Page) *Page {
	p.t.Helper()

	var missing []string

	for _, id := range p.ExtractOOBTargetIDs() {
		if fullPage.Doc.Find("#"+id).Length() == 0 {
			missing = append(missing, fmt.Sprintf("partial OOB target #%s does not exist on full page", id))
		}
	}

	if len(missing) > 0 {
		p.t.Fatalf("OOB target contract failed:\n  - %s", strings.Join(missing, "\n  - "))
	}

	return p
}
