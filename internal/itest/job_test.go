// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"context"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// call is one run of a fake command.
type call struct {
	Dir  string
	Name string
	Args []string
}

// fakeExec records calls and answers them with respond.
type fakeExec struct {
	respond func(ctx context.Context, c call) ([]byte, error)

	mu    sync.Mutex
	calls []call
}

func (f *fakeExec) run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	c := call{Dir: dir, Name: name, Args: slices.Clone(args)}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	return f.respond(ctx, c)
}

func TestExec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "Output of the process", command: "go", args: []string{"env", "GOOS"}, want: runtime.GOOS + "\n"},
		{name: "Missing program", command: "no-such-program-here", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := Exec(t.Context(), t.TempDir(), tc.command, tc.args...)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(out))
		})
	}
}

func TestJobs(t *testing.T) {
	t.Parallel()

	a := Spec{Name: "3.0/a.yml", Path: "/specs/3.0/a.yml", Size: 2}
	b := Spec{Name: "3.1/a.yml", Path: "/specs/3.1/a.yml", Size: 1}
	models := Variant{Name: "models"}
	chi := Variant{Name: "chi", Config: "server: {}\n"}

	got := Jobs([]Spec{a, b}, []Variant{models, chi})

	assert.Equal(t, []Job{
		{Spec: a, Variant: models, Package: "specs/models/s3_0_a"},
		{Spec: a, Variant: chi, Package: "specs/chi/s3_0_a"},
		{Spec: b, Variant: models, Package: "specs/models/s3_1_a"},
		{Spec: b, Variant: chi, Package: "specs/chi/s3_1_a"},
	}, got)
}
