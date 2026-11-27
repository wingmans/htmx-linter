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

For a source scan outside a test, call
`uitest.ScanSourceRules(rootDir, uitest.DefaultLintConfig())`. It returns
findings with a file path, line, column, and message. A scan failure returns an
error. Use `ReportSourceRules` to print findings from a Go test and optionally
treat them as errors.

When developing alongside Chopstick, check out both repositories under the
same parent directory so Chopstick can resolve `../htmx-linter`.
