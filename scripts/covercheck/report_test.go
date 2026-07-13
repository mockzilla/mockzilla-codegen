// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testModule = "example.com/m"

func TestSummarize(t *testing.T) {
	t.Parallel()

	blocks := []block{
		{file: "example.com/m/a/x.go", startLine: 1, endLine: 2, stmts: 2, count: 1},
		{file: "example.com/m/a/x.go", startLine: 4, endLine: 5, stmts: 1, count: 0},
		{file: "example.com/m/b/y.go", startLine: 1, endLine: 1, stmts: 3, count: 7},
		{file: "example.com/m/cmd/main.go", startLine: 1, endLine: 1, stmts: 1, count: 0},
		{file: "example.com/m/empty/z.go", startLine: 1, endLine: 1, stmts: 0, count: 0},
	}

	got := summarize(blocks, testModule, []string{"cmd/main.go"})

	assert.Equal(t, []pkgCoverage{
		{
			pkg:       "example.com/m/a",
			total:     3,
			covered:   2,
			uncovered: []block{blocks[1]},
		},
		{pkg: "example.com/m/b", total: 3, covered: 3},
	}, got)
}

func TestReport(t *testing.T) {
	t.Parallel()

	full := pkgCoverage{pkg: "example.com/m/a", total: 4, covered: 4}
	partial := pkgCoverage{
		pkg:     "example.com/m/b",
		total:   3,
		covered: 2,
		uncovered: []block{
			{file: "example.com/m/b/y.go", startLine: 10, endLine: 12},
		},
	}

	tests := []struct {
		name   string
		pkgs   []pkgCoverage
		min    float64
		want   string
		wantOK bool
	}{
		{
			name:   "All packages at the minimum pass",
			pkgs:   []pkgCoverage{full},
			min:    100,
			want:   "coverage ok: 1 packages at or above 100.0%\n",
			wantOK: true,
		},
		{
			name:   "Package below the minimum fails with its uncovered ranges",
			pkgs:   []pkgCoverage{full, partial},
			min:    100,
			want:   "FAIL example.com/m/b 66.6% (min 100.0%)\n  b/y.go:10-12\n",
			wantOK: false,
		},
		{
			name:   "Lower minimum lets a partial package pass",
			pkgs:   []pkgCoverage{partial},
			min:    60,
			want:   "coverage ok: 1 packages at or above 60.0%\n",
			wantOK: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out strings.Builder
			ok := report(&out, tc.pkgs, tc.min, testModule)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, out.String())
		})
	}
}

func TestFormatPercent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   float64
		want string
	}{
		{name: "Whole number keeps one decimal", in: 100, want: "100.0"},
		{name: "Value just below a hundred is floored", in: 99.96, want: "99.9"},
		{name: "Zero prints as zero", in: 0, want: "0.0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, formatPercent(tc.in))
		})
	}
}
