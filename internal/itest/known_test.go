// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKnown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       string
		want       map[string]Known
		wantErrMsg string
	}{
		{
			name: "Lines with comments and blanks",
			data: "# header\n\n3.0/a.yml generate: bad ref\n  3.1/b.yml build: name clash: twice  \n",
			want: map[string]Known{
				"3.0/a.yml": {Stage: StageGenerate, Reason: "bad ref"},
				"3.1/b.yml": {Stage: StageBuild, Reason: "name clash: twice"},
			},
		},
		{name: "Empty file", want: map[string]Known{}},
		{
			name:       "Unknown stage",
			data:       "3.0/a.yml parse: bad\n",
			wantErrMsg: `bad known-failures line 1: want "<spec> <stage>: <reason>" with stage generate or build, got "3.0/a.yml parse: bad"`,
		},
		{
			name:       "No reason",
			data:       "# c\n3.0/a.yml build:\n",
			wantErrMsg: `bad known-failures line 2: want "<spec> <stage>: <reason>" with stage generate or build, got "3.0/a.yml build:"`,
		},
		{
			name:       "No stage",
			data:       "3.0/a.yml\n",
			wantErrMsg: `bad known-failures line 1: want "<spec> <stage>: <reason>" with stage generate or build, got "3.0/a.yml"`,
		},
		{
			name:       "Listed twice",
			data:       "3.0/a.yml build: x\n3.0/a.yml generate: y\n",
			wantErrMsg: "bad known-failures line 2: 3.0/a.yml is listed twice",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseKnown([]byte(tc.data))

			if tc.wantErrMsg != "" {
				require.ErrorIs(t, err, ErrKnownLine)
				assert.EqualError(t, err, tc.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()

	result := func(name, stage string) Result {
		return Result{Job: Job{Spec: Spec{Name: name}}, Stage: stage}
	}
	known := map[string]Known{
		"listed-pass.yml":  {Stage: StageBuild, Reason: "r"},
		"listed-same.yml":  {Stage: StageBuild, Reason: "r"},
		"listed-other.yml": {Stage: StageBuild, Reason: "r"},
		"not-run.yml":      {Stage: StageBuild, Reason: "r"},
	}
	results := []Result{
		result("pass.yml", ""),
		result("listed-pass.yml", ""),
		result("fail.yml", StageBuild),
		result("listed-same.yml", StageBuild),
		result("listed-other.yml", StageGenerate),
	}

	got := Compare(results, known)

	assert.Equal(t, Verdict{
		New:   []Result{result("fail.yml", StageBuild), result("listed-other.yml", StageGenerate)},
		Fixed: []string{"listed-pass.yml"},
	}, got)
}
