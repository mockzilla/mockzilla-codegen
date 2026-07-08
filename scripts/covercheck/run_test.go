// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testGoMod      = "module example.com/m\n"
	coveredProfile = "mode: atomic\nexample.com/m/a/x.go:1.1,2.2 1 1\n"
	partialProfile = "mode: atomic\nexample.com/m/a/x.go:1.1,2.2 1 1\nexample.com/m/a/x.go:4.1,5.2 1 0\n"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		gomod      *string
		profile    *string
		ignore     *string
		extraArgs  []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "Fully covered profile passes",
			gomod:      new(testGoMod),
			profile:    new(coveredProfile),
			wantCode:   exitOK,
			wantStdout: "coverage ok: 1 packages at or above 100.0%\n",
		},
		{
			name:       "Partly covered profile fails the gate",
			gomod:      new(testGoMod),
			profile:    new(partialProfile),
			wantCode:   exitBelow,
			wantStdout: "FAIL example.com/m/a 50.0% (min 100.0%)\n  a/x.go:4-5\n",
		},
		{
			name:       "Minimum flag lowers the gate",
			gomod:      new(testGoMod),
			profile:    new(partialProfile),
			extraArgs:  []string{"-min", "50"},
			wantCode:   exitOK,
			wantStdout: "coverage ok: 1 packages at or above 50.0%\n",
		},
		{
			name:       "Ignored file does not count",
			gomod:      new(testGoMod),
			profile:    new(partialProfile + "example.com/m/b/main.go:1.1,2.2 1 0\n"),
			ignore:     new("b/main.go\n"),
			extraArgs:  []string{"-min", "50"},
			wantCode:   exitOK,
			wantStdout: "coverage ok: 1 packages at or above 50.0%\n",
		},
		{
			name:       "Unknown flag is a usage error",
			extraArgs:  []string{"-nope"},
			wantCode:   exitError,
			wantStderr: "flag provided but not defined: -nope",
		},
		{
			name:       "Missing go.mod is an error",
			profile:    new(coveredProfile),
			wantCode:   exitError,
			wantStderr: "covercheck: read go.mod",
		},
		{
			name:       "Bad ignore file is an error",
			gomod:      new(testGoMod),
			profile:    new(coveredProfile),
			ignore:     new("[\n"),
			wantCode:   exitError,
			wantStderr: "covercheck: bad ignore pattern",
		},
		{
			name:       "Missing profile is an error",
			gomod:      new(testGoMod),
			wantCode:   exitError,
			wantStderr: "covercheck: read profile",
		},
		{
			name:       "Malformed profile is an error",
			gomod:      new(testGoMod),
			profile:    new("garbage\n"),
			wantCode:   exitError,
			wantStderr: "covercheck: profile has no mode line",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			args := []string{
				"-gomod", writeFile(t, dir, "go.mod", tc.gomod),
				"-profile", writeFile(t, dir, "coverage.out", tc.profile),
				"-ignore", writeFile(t, dir, ".covignore", tc.ignore),
			}
			args = append(args, tc.extraArgs...)

			var stdout, stderr strings.Builder
			code := run(args, &stdout, &stderr)

			assert.Equal(t, tc.wantCode, code)
			assert.Equal(t, tc.wantStdout, stdout.String())
			assert.Contains(t, stderr.String(), tc.wantStderr)
		})
	}
}

func writeFile(t *testing.T, dir, name string, content *string) string {
	t.Helper()

	file := filepath.Join(dir, name)
	if content != nil {
		require.NoError(t, os.WriteFile(file, []byte(*content), 0o600))
	}
	return file
}
