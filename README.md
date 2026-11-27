# htmx-linter

Go test helpers for linting rendered HTMX pages and fragments, checking
out-of-band swap targets, and scanning HTML, templ templates, and CSS sources.
The module requires Go 1.26 or later.

Import `github.com/wingmans/htmx-linter` in a Go test. The package name is
`uitest`.

```go
cfg := uitest.DefaultLintConfig()
uitest.NewHarness(t, handler).GetPage("/").AssertStatus(http.StatusOK).Lint(cfg)
```

When developing alongside Chopstick, check out both repositories under the
same parent directory so Chopstick can resolve `../htmx-linter`.
