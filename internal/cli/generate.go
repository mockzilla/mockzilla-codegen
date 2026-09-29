// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/mockzilla/mockzilla-codegen/pkg/codegen"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// generateFlags are the flags of the generate command.
type generateFlags struct {
	configPath string
	isDryRun   bool
	isCheck    bool
	isVerbose  bool
}

// output prints what generate did, with paths relative to the config folder.
type output struct {
	dir    string
	stdout io.Writer
	stderr io.Writer
}

func generate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var gf generateFlags
	set := flag.NewFlagSet("generate", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.StringVar(&gf.configPath, "c", "codegen.yaml", "config file")
	set.BoolVar(&gf.isDryRun, "dry-run", false, "print the files and what would happen to each, write nothing")
	set.BoolVar(&gf.isCheck, "check", false, "exit 1 when a generated file is missing or differs")
	set.BoolVar(&gf.isVerbose, "v", false, "also print info diagnostics and the files written")

	specs, err := parseFlags(set, args)
	switch {
	case err != nil:
		return usageCode(err)
	case len(specs) > 1:
		_, _ = fmt.Fprintf(stderr, "%s: generate takes at most one spec, got %d\n", program, len(specs))
		return ExitUsage
	case gf.isDryRun && gf.isCheck:
		_, _ = fmt.Fprintf(stderr, "%s: -dry-run and -check do not go together\n", program)
		return ExitUsage
	}

	cfg, err := config.Load(gf.configPath)
	if err == nil && len(specs) == 1 {
		// Abs fails only when the working directory is gone, so it shares the load error path.
		cfg.Spec.Path, err = filepath.Abs(specs[0])
	}
	if err != nil {
		return fail(stderr, err)
	}

	res, err := codegen.Generate(ctx, cfg)
	if err != nil {
		return fail(stderr, err)
	}
	out := &output{dir: cfg.Resolve("."), stdout: stdout, stderr: stderr}
	out.diagnostics(res.Diagnostics, gf.isVerbose)

	switch {
	case gf.isCheck:
		return out.check(res)
	case gf.isDryRun:
		return out.write(res, codegen.WriteOptions{DryRun: true}, true)
	default:
		isOverwrite := cfg.Server != nil && cfg.Server.Scaffold.Overwrite
		return out.write(res, codegen.WriteOptions{OverwriteScaffolds: isOverwrite}, gf.isVerbose)
	}
}

// diagnostics prints warnings and errors, and info too when verbose.
func (o *output) diagnostics(list []codegen.Diagnostic, isVerbose bool) {
	for _, d := range list {
		if d.Severity == codegen.SeverityInfo && !isVerbose {
			continue
		}

		var b strings.Builder
		if d.File != "" {
			b.WriteString(o.rel(d.File))
			if d.Line > 0 {
				b.WriteString(":" + strconv.Itoa(d.Line) + ":" + strconv.Itoa(d.Col))
			}
			b.WriteString(": ")
		}
		b.WriteString(d.Severity.String() + " " + d.Code + ": " + d.Message)
		if d.Pointer != "" {
			b.WriteString(" [" + d.Pointer + "]")
		}
		_, _ = fmt.Fprintln(o.stderr, b.String())
	}
}

// write writes the files, or only reports them on a dry run, and prints a table when asked.
func (o *output) write(res *codegen.Result, opts codegen.WriteOptions, isTable bool) int {
	reports, err := codegen.Write(res, opts)
	if err != nil {
		return fail(o.stderr, err)
	}
	if !isTable {
		return ExitOK
	}

	tw := tabwriter.NewWriter(o.stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FILE\tPACKAGE\tPARTS\tACTION")
	for i, f := range res.Files {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", o.rel(f.Path), f.Package, strings.Join(f.Parts, ","), reports[i].Action)
	}
	_ = tw.Flush()
	return ExitOK
}

// check lists the generated files that are missing or differ from what Generate made. Scaffolds
// are the user's once written, so they are not checked.
func (o *output) check(res *codegen.Result) int {
	var stale []string
	for _, f := range res.Files {
		if f.Kind == codegen.FileScaffold {
			continue
		}
		data, err := os.ReadFile(f.Path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			stale = append(stale, o.rel(f.Path)+": missing")
		case err != nil:
			return fail(o.stderr, err)
		case !bytes.Equal(data, f.Content):
			stale = append(stale, o.rel(f.Path)+": differs")
		}
	}

	if len(stale) == 0 {
		return ExitOK
	}
	_, _ = fmt.Fprintf(o.stdout, "generated files are out of date, run %s generate:\n", program)
	for _, line := range stale {
		_, _ = fmt.Fprintln(o.stdout, "  "+line)
	}
	return ExitFail
}

// rel returns p relative to the config folder when it is inside it.
func (o *output) rel(p string) string {
	if r, err := filepath.Rel(o.dir, p); err == nil && filepath.IsLocal(r) {
		return r
	}
	return p
}
