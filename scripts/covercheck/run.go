package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const (
	exitOK = iota
	exitBelow
	exitError
)

type options struct {
	profile string
	minPct  float64
	ignore  string
	gomod   string
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("covercheck", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var opts options
	flags.StringVar(&opts.profile, "profile", "coverage.out", "coverage profile to check")
	flags.Float64Var(&opts.minPct, "min", 100, "minimum coverage per package, in percent")
	flags.StringVar(&opts.ignore, "ignore", ".covignore", "file listing module-relative paths to skip")
	flags.StringVar(&opts.gomod, "gomod", "go.mod", "go.mod of the module under test")

	if err := flags.Parse(args); err != nil {
		return exitError
	}

	err := check(opts, stdout)
	if errors.Is(err, errBelowMinimum) {
		return exitBelow
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "covercheck: %v\n", err)
		return exitError
	}

	return exitOK
}

func check(opts options, w io.Writer) error {
	module, err := modulePath(opts.gomod)
	if err != nil {
		return err
	}

	patterns, err := loadIgnore(opts.ignore)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(opts.profile)
	if err != nil {
		return fmt.Errorf("read profile: %w", err)
	}

	blocks, err := parseProfile(bytes.NewReader(data))
	if err != nil {
		return err
	}

	if !report(w, summarize(blocks, module, patterns), opts.minPct, module) {
		return errBelowMinimum
	}

	return nil
}
