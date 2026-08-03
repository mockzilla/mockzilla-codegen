// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/codegen/pkg/config"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("closed pipe")
}

// run runs the command line and returns its exit code, stdout and stderr.
func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr strings.Builder
	code := Run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRun(t *testing.T) {
	t.Parallel()

	schema, err := config.Schema()
	require.NoError(t, err)

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "No command prints the usage", wantCode: ExitUsage, wantStderr: usage},
		{name: "Help prints the usage", args: []string{"help"}, wantStdout: usage},
		{name: "Help flag prints the usage", args: []string{"--help"}, wantStdout: usage},
		{
			name:       "Unknown command",
			args:       []string{"build"},
			wantCode:   ExitUsage,
			wantStderr: "mockzilla-codegen: unknown command \"build\"\n\n" + usage,
		},
		{name: "Version", args: []string{"version"}, wantStdout: "dev\n"},
		{name: "Schema", args: []string{"schema"}, wantStdout: string(schema)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			code, stdout, stderr := run(t, tc.args...)

			assert.Equal(t, tc.wantCode, code)
			assert.Equal(t, tc.wantStdout, stdout)
			assert.Equal(t, tc.wantStderr, stderr)
		})
	}
}

func TestRunSchemaWriteError(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	code := Run(context.Background(), []string{"schema"}, failWriter{}, &stderr)

	assert.Equal(t, ExitFail, code)
	assert.Equal(t, "mockzilla-codegen: closed pipe\n", stderr.String())
}

func TestParseFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantRest []string
		wantFlag bool
		wantErr  error
	}{
		{name: "Flags before arguments", args: []string{"-x", "a", "b"}, wantRest: []string{"a", "b"}, wantFlag: true},
		{name: "Flags after arguments", args: []string{"a", "-x", "b"}, wantRest: []string{"a", "b"}, wantFlag: true},
		{name: "No arguments", args: []string{"-x"}, wantFlag: true},
		{name: "Unknown flag", args: []string{"a", "-y"}, wantErr: errors.New("flag provided but not defined: -y")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			set := flag.NewFlagSet("test", flag.ContinueOnError)
			set.SetOutput(io.Discard)
			isSet := set.Bool("x", false, "")

			rest, err := parseFlags(set, tc.args)

			if tc.wantErr != nil {
				assert.EqualError(t, err, tc.wantErr.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantRest, rest)
			assert.Equal(t, tc.wantFlag, *isSet)
		})
	}
}
