// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReport(t *testing.T) {
	t.Parallel()

	job := func(name string) Job {
		return Job{Spec: Spec{Name: name}, Variant: Variant{Name: "models"}}
	}
	long := strings.Repeat("x", 201)
	var output []string
	output = append(output, long)
	for i := 2; i <= 17; i++ {
		output = append(output, fmt.Sprintf("l%d", i))
	}
	tests := []struct {
		name    string
		results []Result
		want    string
	}{
		{name: "Empty run", want: "0 jobs, 0 cached: 0 passed, 0 failed\ngenerated 0 lines in 0 jobs\n"},
		{
			name: "Failures and cached jobs",
			results: []Result{
				{Job: job("3.0/a.yml"), Lines: 100, Elapsed: 2 * time.Second},
				{Job: job("3.0/c.yml"), IsCached: true},
				{Job: job("3.0/b.yml"), Stage: StageBuild, Output: "err1\nerr2", Lines: 50, Elapsed: time.Second},
				{Job: job("3.1/d.yml"), Stage: StageGenerate, Output: strings.Join(output, "\n"), Elapsed: 3 * time.Second},
				{Job: job("3.1/e.yml"), Lines: 10, Elapsed: 500 * time.Millisecond},
			},
			want: "5 jobs, 1 cached: 3 passed, 2 failed\n" +
				"failed at build: 1\n" +
				"failed at generate: 1\n" +
				"generated 160 lines in 4 jobs\n" +
				"\nslowest to generate:\n" +
				"        3s  3.1/d.yml (models)\n" +
				"        2s  3.0/a.yml (models)\n" +
				"        1s  3.0/b.yml (models)\n" +
				"     500ms  3.1/e.yml (models)\n" +
				"\nfailures, first 2 of 2:\n" +
				"\n3.0/b.yml (models) at build\n" +
				"    err1\n    err2\n" +
				"\n3.1/d.yml (models) at generate\n" +
				"    " + long[:200] + "...\n" +
				"    l2\n    l3\n    l4\n    l5\n    l6\n    l7\n    l8\n    l9\n    l10\n    l11\n    l12\n    l13\n    l14\n    l15\n" +
				"    ... 2 more lines\n" +
				"\nfailed:\n" +
				"  3.0/b.yml (models) at build\n" +
				"  3.1/d.yml (models) at generate\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Report(tc.results))
		})
	}
}

func TestReportCapsLongLists(t *testing.T) {
	t.Parallel()

	var results []Result
	for i := range 60 {
		name := fmt.Sprintf("s%02d.yml", i)
		results = append(results, Result{
			Job:     Job{Spec: Spec{Name: name}, Variant: Variant{Name: "models"}},
			Stage:   StageBuild,
			Output:  "bad",
			Elapsed: time.Duration(i) * time.Millisecond,
		})
	}

	got := Report(results)

	assert.Contains(t, got, "\nfailures, first 50 of 60:\n")
	assert.Equal(t, 50, strings.Count(got, "    bad\n"))
	assert.Equal(t, 60, strings.Count(got, "\n  s"))
	assert.Contains(t, got, "     50ms  s50.yml (models)\n")
	assert.NotContains(t, got, "     49ms  s49.yml (models)\n")
}
