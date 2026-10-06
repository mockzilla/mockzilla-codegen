// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package cli is the mockzilla-codegen command line. It is the only place that prints.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/mockzilla/mockzilla-codegen/pkg/codegen"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// program is the name of the executable, the folder of its main package.
const program = "mockzilla-codegen"

// Exit codes.
const (
	ExitOK    = 0
	ExitFail  = 1
	ExitUsage = 2
)

const usage = `Usage: mockzilla-codegen <command> [flags]

Commands:
  generate [-c codegen.yaml] [-dry-run | -check] [-strict] [-v] [flags] [spec]
            Generate the files the config lists. A spec argument replaces spec.path.
            Without a config file the defaults hold: models only, in ./gen.go.
            -server <framework>, -client and -mcp turn a part on, -no-server,
            -no-client and -no-mcp turn it off, -o and -package set the output.
            Run mockzilla-codegen generate -h for every flag.
  schema    Print the JSON schema of the config file.
  version   Print the mockzilla-codegen version.
`

// Run runs the command line with args, the program name left out, and returns the exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return ExitUsage
	}

	switch args[0] {
	case "generate":
		return generate(ctx, args[1:], stdout, stderr)
	case "schema":
		return schema(stdout, stderr)
	case "version":
		_, _ = fmt.Fprintln(stdout, codegen.Version())
		return ExitOK
	case "help", "-h", "-help", "--help":
		_, _ = fmt.Fprint(stdout, usage)
		return ExitOK
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown command %q\n\n%s", program, args[0], usage)
		return ExitUsage
	}
}

func schema(stdout, stderr io.Writer) int {
	data, err := config.Schema()
	if err == nil {
		_, err = stdout.Write(data)
	}
	if err != nil {
		return fail(stderr, err)
	}
	return ExitOK
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

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "%s: %v\n", program, err)
	return ExitFail
}

func misuse(stderr io.Writer, message string) int {
	_, _ = fmt.Fprintf(stderr, "%s: %s\n", program, message)
	return ExitUsage
}

// plural writes n with the noun, which takes an s unless n is 1.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
