// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package cli

import (
	"bytes"
	"cmp"
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

	"github.com/mockzilla/mockzilla-codegen/internal/bundle"
	"github.com/mockzilla/mockzilla-codegen/pkg/codegen"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// defaultConfig is the config file generate reads when -c is not given, if it exists.
const defaultConfig = "codegen.yaml"

// generateFlags are the flags of the generate command, and the spec argument.
type generateFlags struct {
	configPath  string
	specPath    string
	outputFile  string
	packageName string
	framework   string
	isDryRun    bool
	isCheck     bool
	isStrict    bool
	isVerbose   bool
	isClient    bool
	isMCP       bool
	isNoServer  bool
	isNoClient  bool
	isNoMCP     bool
}

// load reads the config, or takes the defaults when -c is not given and no codegen.yaml exists.
func (gf *generateFlags) load(wd string) (*config.Config, error) {
	if gf.specPath != "" && !bundle.IsURL(gf.specPath) {
		gf.specPath = inDir(wd, gf.specPath)
	}
	if gf.outputFile != "" {
		gf.outputFile = inDir(wd, gf.outputFile)
	}

	cfg, err := config.Load(cmp.Or(gf.configPath, defaultConfig), gf.edit)
	switch {
	case gf.configPath != "" || !errors.Is(err, fs.ErrNotExist):
		return cfg, err
	case gf.specPath == "":
		return nil, errNoSpec
	default:
		return config.Parse(nil, wd, gf.edit)
	}
}

// edit makes the changes of the flags and the spec argument, before the defaults are filled in.
func (gf *generateFlags) edit(c *config.Config) {
	c.Spec.Path = cmp.Or(gf.specPath, c.Spec.Path)
	c.Output.File = cmp.Or(gf.outputFile, c.Output.File)
	c.Package = cmp.Or(gf.packageName, c.Package)

	if gf.framework != "" {
		c.Server = cmp.Or(c.Server, &config.Server{})
		c.Server.Framework = gf.framework
	}
	if gf.isClient {
		c.Client = cmp.Or(c.Client, &config.Client{})
	}
	if gf.isMCP {
		c.MCP = cmp.Or(c.MCP, &config.MCP{})
	}

	if gf.isNoServer {
		c.Server = nil
	}
	if gf.isNoClient {
		c.Client = nil
	}
	if gf.isNoMCP {
		c.MCP = nil
	}
}

// output prints what generate did, with paths relative to the config folder.
type output struct {
	dir    string
	stdout io.Writer
	stderr io.Writer
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

// verdict fails the run when an error was printed, or a warning with -strict.
func (o *output) verdict(list []codegen.Diagnostic, isStrict bool) int {
	var errs, warnings int
	for _, d := range list {
		switch d.Severity {
		case codegen.SeverityError:
			errs++
		case codegen.SeverityWarning:
			warnings++
		case codegen.SeverityInfo:
		}
	}
	if !isStrict {
		warnings = 0
	}

	var counts []string
	if errs > 0 {
		counts = append(counts, plural(errs, "error"))
	}
	if warnings > 0 {
		counts = append(counts, plural(warnings, "warning"))
	}
	if len(counts) == 0 {
		return ExitOK
	}
	_, _ = fmt.Fprintf(o.stderr, "%s: failed on %s\n", program, strings.Join(counts, " and "))
	return ExitFail
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

func generate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var gf generateFlags
	set := flag.NewFlagSet("generate", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.StringVar(&gf.configPath, "c", "", "config file; without it, "+defaultConfig+" when it exists, else the defaults")
	set.BoolVar(&gf.isDryRun, "dry-run", false, "print the files and what would happen to each, write nothing")
	set.BoolVar(&gf.isCheck, "check", false, "exit 1 when a generated file is missing or differs")
	set.BoolVar(&gf.isStrict, "strict", false, "also exit 1 when a warning is printed")
	set.BoolVar(&gf.isVerbose, "v", false, "also print info diagnostics and the files written")
	set.StringVar(&gf.framework, "server", "", "generate a server for this framework")
	set.BoolVar(&gf.isClient, "client", false, "generate a client")
	set.BoolVar(&gf.isMCP, "mcp", false, "generate MCP tools, which need a client")
	set.BoolVar(&gf.isNoServer, "no-server", false, "generate no server, whatever the config says")
	set.BoolVar(&gf.isNoClient, "no-client", false, "generate no client, whatever the config says")
	set.BoolVar(&gf.isNoMCP, "no-mcp", false, "generate no MCP tools, whatever the config says")
	set.StringVar(&gf.outputFile, "o", "", "output.file, relative to the current folder")
	set.StringVar(&gf.packageName, "package", "", "package of the output file")

	specs, err := parseFlags(set, args)
	switch {
	case err != nil:
		return usageCode(err)
	case len(specs) > 1:
		return misuse(stderr, fmt.Sprintf("generate takes at most one spec, got %d", len(specs)))
	case gf.isDryRun && gf.isCheck:
		return misuse(stderr, "-dry-run and -check do not go together")
	case gf.framework != "" && gf.isNoServer:
		return misuse(stderr, "-server and -no-server do not go together")
	case gf.isClient && gf.isNoClient:
		return misuse(stderr, "-client and -no-client do not go together")
	case gf.isMCP && gf.isNoMCP:
		return misuse(stderr, "-mcp and -no-mcp do not go together")
	}
	if len(specs) == 1 {
		gf.specPath = specs[0]
	}

	var cfg *config.Config
	// Getwd fails only when the working directory is gone, so it shares the load error path.
	wd, err := os.Getwd()
	if err == nil {
		cfg, err = gf.load(wd)
	}
	switch {
	case errors.Is(err, errNoSpec):
		return misuse(stderr, err.Error())
	case err != nil:
		return fail(stderr, err)
	}

	res, err := codegen.Generate(ctx, cfg)
	if err != nil {
		return fail(stderr, err)
	}
	out := &output{dir: cfg.Resolve("."), stdout: stdout, stderr: stderr}
	out.diagnostics(res.Diagnostics, gf.isVerbose)

	var code int
	switch {
	case gf.isCheck:
		code = out.check(res)
	case gf.isDryRun:
		code = out.write(res, codegen.WriteOptions{DryRun: true}, true)
	default:
		isOverwrite := cfg.Server != nil && cfg.Server.Scaffold.Overwrite
		code = out.write(res, codegen.WriteOptions{OverwriteScaffolds: isOverwrite}, gf.isVerbose)
	}
	if code != ExitOK {
		return code
	}
	return out.verdict(res.Diagnostics, gf.isStrict)
}

// inDir returns p joined with dir, unless p is absolute.
func inDir(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}
