// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

const (
	shownFailures = 50
	shownLines    = 15
	lineWidth     = 200
	shownSlowest  = 10
)

// Report is the text summary of a run: totals, failures by stage, generated lines, the slowest
// jobs, the first failures with their output, and every failed job.
func Report(results []Result, known map[string]Known) string {
	var failed, slow []Result
	cached, lines, fresh := 0, 0, 0
	byStage := map[string]int{}
	for _, r := range results {
		if r.Stage != "" {
			failed = append(failed, r)
			byStage[r.Stage]++
		}
		if r.IsCached {
			cached++
			continue
		}
		lines += r.Lines
		fresh++
		slow = append(slow, r)
	}
	v := Compare(results, known)

	var b strings.Builder
	fmt.Fprintf(&b, "%d jobs, %d cached: %d passed, %d failed (%d new), %d fixed\n",
		len(results), cached, len(results)-len(failed), len(failed), len(v.New), len(v.Fixed))
	for _, stage := range slices.Sorted(maps.Keys(byStage)) {
		fmt.Fprintf(&b, "failed at %s: %d\n", stage, byStage[stage])
	}
	fmt.Fprintf(&b, "generated %d lines in %d jobs\n", lines, fresh)

	slices.SortStableFunc(slow, func(a, c Result) int { return cmp.Compare(c.Elapsed, a.Elapsed) })
	if len(slow) > 0 {
		b.WriteString("\nslowest to generate:\n")
	}
	for _, r := range slow[:min(shownSlowest, len(slow))] {
		fmt.Fprintf(&b, "  %8s  %s\n", r.Elapsed.Round(time.Millisecond), label(r))
	}

	slices.SortStableFunc(failed, func(a, c Result) int {
		return cmp.Or(cmp.Compare(rank(a, known), rank(c, known)), cmp.Compare(label(a), label(c)))
	})
	if len(failed) > 0 {
		fmt.Fprintf(&b, "\nfailures, first %d of %d:\n", min(shownFailures, len(failed)), len(failed))
	}
	for _, r := range failed[:min(shownFailures, len(failed))] {
		fmt.Fprintf(&b, "\n%s at %s%s\n", label(r), r.Stage, newMark(r, known))
		writeOutput(&b, r.Output)
	}

	if len(failed) > 0 {
		b.WriteString("\nfailed:\n")
	}
	for _, r := range failed {
		fmt.Fprintf(&b, "  %s at %s%s\n", label(r), r.Stage, newMark(r, known))
	}
	if len(v.Fixed) > 0 {
		b.WriteString("\npassing now, remove from the known failures:\n")
	}
	for _, name := range v.Fixed {
		b.WriteString("  " + name + "\n")
	}
	return b.String()
}

func label(r Result) string {
	return r.Job.Spec.Name + " (" + r.Job.Variant.Name + ")"
}

func newMark(r Result, known map[string]Known) string {
	if isNew(r, known) {
		return " [new]"
	}
	return ""
}

// writeOutput writes the first lines of out, indented and cut to lineWidth.
func writeOutput(b *strings.Builder, out string) {
	lines := strings.Split(out, "\n")
	for _, line := range lines[:min(shownLines, len(lines))] {
		if len(line) > lineWidth {
			line = line[:lineWidth] + "..."
		}
		b.WriteString("    " + line + "\n")
	}
	if len(lines) > shownLines {
		fmt.Fprintf(b, "    ... %d more lines\n", len(lines)-shownLines)
	}
}

// rank sorts new failures before known ones.
func rank(r Result, known map[string]Known) int {
	if isNew(r, known) {
		return 0
	}
	return 1
}
