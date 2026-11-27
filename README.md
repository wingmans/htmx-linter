# htmx-linter

Go test helpers for linting rendered HTMX pages and fragments, checking
out-of-band swap targets, and scanning HTML, templ templates, and CSS sources.
The module requires Go 1.26 or later.

## Command line

Run `make` to see the available targets. `make build` builds
`bin/htmx-linter`. `make run` builds it and scans the current directory by
default. Set `DIR` to scan a different directory.

```sh
make run DIR=../chopstick/internal/chopstick/web
make run DIR=../chopstick/internal/chopstick/web FLAGS=-warnings-as-errors
```

Findings appear on stderr as `file:line:column: warning: message`.
Use `-warnings-as-errors` to report findings as errors and fail the check.
The built executable exits with status 0 for a successful scan in warning mode
or a clean scan in strict mode, 1 for findings in strict mode, and 2 for invalid
arguments or a scan failure.

## Go API

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
