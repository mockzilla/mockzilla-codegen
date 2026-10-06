// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package cli is the mockzilla-codegen command line, for its own binary and for programs that
// run it as one of their commands. It is the only place that prints.
package cli

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/mockzilla/mockzilla-codegen/pkg/codegen"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// defaultName is the name of the executable, the folder of its main package.
const defaultName = "mockzilla-codegen"

// Exit codes.
const (
	ExitOK    = 0
	ExitFail  = 1
	ExitUsage = 2
)

const usageFormat = `Usage: %[1]s <command> [flags]

Commands:
  generate [-c codegen.yaml] [-dry-run | -check] [-strict] [-v] [flags] [spec]
            Generate the files the config lists. A spec argument replaces spec.path.
            Without a config file the defaults hold: models only, in ./gen.go.
            -server <framework>, -client and -mcp turn a part on, -no-server,
            -no-client and -no-mcp turn it off, -o and -package set the output.
            Run %[1]s generate -h for every flag.
  schema    Print the JSON schema of the config file.
  version   Print the mockzilla-codegen version.
`

// Command is the command line. It writes to Stdout and Stderr only.
type Command struct {
	// Name is what usage and messages call the program, mockzilla-codegen when empty.
	Name   string
	Stdout io.Writer
	Stderr io.Writer
}

// Run runs the command line with args, the program name left out, and returns the exit code.
func (c *Command) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.Stderr, c.usage())
		return ExitUsage
	}

	switch args[0] {
	case "generate":
		return c.generate(ctx, args[1:])
	case "schema":
		return c.schema()
	case "version":
		_, _ = fmt.Fprintln(c.Stdout, codegen.Version())
		return ExitOK
	case "help", "-h", "-help", "--help":
		_, _ = fmt.Fprint(c.Stdout, c.usage())
		return ExitOK
	default:
		_, _ = fmt.Fprintf(c.Stderr, "%s: unknown command %q\n\n%s", c.name(), args[0], c.usage())
		return ExitUsage
	}
}

func (c *Command) schema() int {
	data, err := config.Schema()
	if err == nil {
		_, err = c.Stdout.Write(data)
	}
	if err != nil {
		return c.fail(err)
	}
	return ExitOK
}

func (c *Command) usage() string {
	return fmt.Sprintf(usageFormat, c.name())
}

func (c *Command) fail(err error) int {
	_, _ = fmt.Fprintf(c.Stderr, "%s: %v\n", c.name(), err)
	return ExitFail
}

func (c *Command) misuse(message string) int {
	_, _ = fmt.Fprintf(c.Stderr, "%s: %s\n", c.name(), message)
	return ExitUsage
}

func (c *Command) name() string {
	return cmp.Or(c.Name, defaultName)
}

// parseFlags parses args with fs and returns the arguments that are not flags. Flags may follow
// them.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return rest, nil
		}
		rest = append(rest, args[0])
		args = args[1:]
	}
}

// usageCode is the exit code for a flag parse error: asking for help is not a failure.
func usageCode(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return ExitOK
	}
	return ExitUsage
}

// plural writes n with the noun, which takes an s unless n is 1.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
