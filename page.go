package uitest

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// Page wraps a parsed HTML page or HTMX partial fragment with test helpers.
type Page struct {
	t      *testing.T
	Status int
	Header http.Header
	Body   string
	Doc    *goquery.Document
}

// ParsePage parses raw HTML into a Page so Lint and OOB helpers can inspect it.
func ParsePage(t *testing.T, status int, header http.Header, body string) *Page {
	t.Helper()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed parsing HTML response: %v", err)
	}

	if header == nil {
		header = make(http.Header)
	}

	return &Page{
		t:      t,
		Status: status,
		Header: header,
		Body:   body,
		Doc:    doc,
	}
}

func (p *Page) AssertStatus(want int) *Page {
	p.t.Helper()

	if p.Status != want {
		p.t.Fatalf("unexpected status: got %d, want %d\nbody:\n%s", p.Status, want, p.Body)
	}

	return p
}

func (p *Page) AssertHeader(key, want string) *Page {
	p.t.Helper()

	if got := p.Header.Get(key); got != want {
		p.t.Fatalf("unexpected header %s: got %q, want %q", key, got, want)
	}

	return p
}

// Harness executes requests against an http.Handler in memory without network sockets.
type Harness struct {
	t       *testing.T
	handler http.Handler
}

func NewHarness(t *testing.T, handler http.Handler) *Harness {
	t.Helper()

	return &Harness{t: t, handler: handler}
}

func (h *Harness) GetPage(path string) *Page {
	h.t.Helper()
	req := httptest.NewRequestWithContext(h.t.Context(), http.MethodGet, path, nil)

	return h.do(req)
}

func (h *Harness) HTMXGet(path, sourceID string) *Page {
	h.t.Helper()
	req := httptest.NewRequestWithContext(h.t.Context(), http.MethodGet, path, nil)
	req.Header.Set("Hx-Request", "true")

	if sourceID != "" {
		req.Header.Set("Hx-Source", sourceID)
	}

	return h.do(req)
}

func (h *Harness) HTMXPostForm(path, sourceID string, form url.Values) *Page {
	h.t.Helper()
	req := httptest.NewRequestWithContext(h.t.Context(), http.MethodPost, path, bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Hx-Request", "true")

	if sourceID != "" {
		req.Header.Set("Hx-Source", sourceID)
	}

	return h.do(req)
}

func (h *Harness) do(req *http.Request) *Page {
	h.t.Helper()

	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)

	return ParsePage(h.t, rec.Code, rec.Header().Clone(), rec.Body.String())
}
