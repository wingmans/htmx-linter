package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	uitest "github.com/wingmans/htmx-linter"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("htmx-linter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	strict := flags.Bool("warnings-as-errors", false, "fail when lint findings are reported")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: htmx-linter [options] <directory>")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err == flag.ErrHelp {
		return 0
	} else if err != nil {
		return 2
	}

	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}

	findings, err := uitest.ScanSourceRules(flags.Arg(0), uitest.DefaultLintConfig())
	if err != nil {
		fmt.Fprintf(stderr, "htmx-linter: %v\n", err)
		return 2
	}

	level := "warning"
	if *strict {
		level = "error"
	}

	for _, finding := range findings {
		fmt.Fprintf(stderr, "%s:%d:%d: %s: %s\n", finding.Path, finding.Line, finding.Column, level, finding.Message)
	}

	if *strict && len(findings) > 0 {
		return 1
	}

	return 0
}
