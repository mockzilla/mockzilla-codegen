// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errExit = errors.New("exit status 1")

// generator stands in for the CLI: spec is the spec file name without its extension.
type generator func(ctx context.Context, dir, spec string) ([]byte, error)

// builder stands in for go build: pkgs are the package names of the batch.
type builder func(pkgs []string) ([]byte, error)

func TestRunnerRun(t *testing.T) {
	t.Parallel()

	specs := []Spec{{Name: "a.yml", Path: "/specs/a.yml"}, {Name: "b.yml", Path: "/specs/b.yml"}, {Name: "c.yml", Path: "/specs/c.yml"}}
	jobs := Jobs(specs, []Variant{{Name: "models", Config: "models: {}\n"}})
	pass := func(i int) Result { return Result{Job: jobs[i], Lines: 3} }
	fail := func(i int, stage, out string, lines int) Result {
		return Result{Job: jobs[i], Stage: stage, Output: out, Lines: lines}
	}
	onB := func(gen generator) generator {
		return func(ctx context.Context, dir, spec string) ([]byte, error) {
			if spec == "b" {
				return gen(ctx, dir, spec)
			}
			return writeGen(ctx, dir, spec)
		}
	}

	tests := []struct {
		name         string
		batchSize    int
		prepare      func(t *testing.T, sandbox string)
		gen          generator
		build        builder
		want         func(sandbox string) []Result
		wantBuilds   int
		wantProgress string
	}{
		{
			name:         "Every job passes",
			batchSize:    2,
			want:         func(string) []Result { return []Result{pass(0), pass(1), pass(2)} },
			wantBuilds:   2,
			wantProgress: "3/3 done, 0 failed, 0 generating, 0 building",
		},
		{
			name: "Generation failure",
			gen: onB(func(context.Context, string, string) ([]byte, error) {
				return []byte("bad spec\n"), errExit
			}),
			want: func(string) []Result {
				return []Result{pass(0), fail(1, StageGenerate, "bad spec\nexit status 1", 0), pass(2)}
			},
			wantBuilds:   1,
			wantProgress: "3/3 done, 1 failed, 0 generating, 0 building",
		},
		{
			name: "Generation timeout",
			gen: onB(func(ctx context.Context, _, _ string) ([]byte, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}),
			want: func(string) []Result {
				return []Result{pass(0), fail(1, StageGenerate, "timed out after 200ms", 0), pass(2)}
			},
			wantBuilds:   1,
			wantProgress: "3/3 done, 1 failed, 0 generating, 0 building",
		},
		{
			name: "Build failure named in the output",
			build: func(pkgs []string) ([]byte, error) {
				if slices.Contains(pkgs, "b") {
					return []byte("# sandbox/specs/models/b\nspecs/models/b/gen.go:1:1: bad\n"), errExit
				}
				return nil, nil
			},
			want: func(string) []Result {
				return []Result{pass(0), fail(1, StageBuild, "specs/models/b/gen.go:1:1: bad", 3), pass(2)}
			},
			wantBuilds:   2,
			wantProgress: "3/3 done, 1 failed, 0 generating, 0 building",
		},
		{
			name: "Build failure that names no package is split",
			build: func(pkgs []string) ([]byte, error) {
				switch {
				case !slices.Contains(pkgs, "b"):
					return nil, nil
				case len(pkgs) > 1:
					return []byte("go: something is off\n"), errExit
				default:
					return []byte("go: b is off\n"), errExit
				}
			},
			want: func(string) []Result {
				return []Result{pass(0), fail(1, StageBuild, "go: b is off\nexit status 1", 3), pass(2)}
			},
			wantBuilds:   4,
			wantProgress: "3/3 done, 1 failed, 0 generating, 0 building",
		},
		{
			name: "Config file cannot be written",
			prepare: func(t *testing.T, sandbox string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(sandbox, "specs", "models", "b", "codegen.yml"), 0o755))
			},
			want: func(sandbox string) []Result {
				p := filepath.Join(sandbox, "specs", "models", "b", "codegen.yml")
				return []Result{pass(0), fail(1, StageGenerate, "open "+p+": is a directory", 0), pass(2)}
			},
			wantBuilds:   1,
			wantProgress: "3/3 done, 1 failed, 0 generating, 0 building",
		},
		{
			name: "Package folder cannot be made",
			prepare: func(t *testing.T, sandbox string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(sandbox, "specs"), nil, 0o644))
			},
			want: func(sandbox string) []Result {
				out := "mkdir " + filepath.Join(sandbox, "specs") + ": not a directory"
				return []Result{fail(0, StageGenerate, out, 0), fail(1, StageGenerate, out, 0), fail(2, StageGenerate, out, 0)}
			},
			wantProgress: "3/3 done, 3 failed, 0 generating, 0 building",
		},
		{
			name: "Generated folder is gone",
			gen: onB(func(_ context.Context, dir, _ string) ([]byte, error) {
				return nil, os.RemoveAll(dir)
			}),
			want: func(sandbox string) []Result {
				out := "open " + filepath.Join(sandbox, "specs", "models", "b") + ": no such file or directory"
				return []Result{pass(0), fail(1, StageGenerate, out, 0), pass(2)}
			},
			wantBuilds:   1,
			wantProgress: "3/3 done, 1 failed, 0 generating, 0 building",
		},
		{
			name: "Generated file cannot be read",
			gen: onB(func(_ context.Context, dir, _ string) ([]byte, error) {
				return nil, os.Mkdir(filepath.Join(dir, "x.go"), 0o755)
			}),
			want: func(sandbox string) []Result {
				out := "read " + filepath.Join(sandbox, "specs", "models", "b", "x.go") + ": is a directory"
				return []Result{pass(0), fail(1, StageGenerate, out, 0), pass(2)}
			},
			wantBuilds:   1,
			wantProgress: "3/3 done, 1 failed, 0 generating, 0 building",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sandbox := t.TempDir()
			if tc.prepare != nil {
				tc.prepare(t, sandbox)
			}
			gen, build := writeGen, buildOK
			if tc.gen != nil {
				gen = tc.gen
			}
			if tc.build != nil {
				build = tc.build
			}
			batchSize := 3
			if tc.batchSize > 0 {
				batchSize = tc.batchSize
			}
			f := &fakeExec{respond: respond(gen, build)}
			r := &Runner{
				Exec:        f.run,
				Sandbox:     Sandbox{Dir: sandbox},
				Tool:        "/bin/codegen",
				Timeout:     200 * time.Millisecond,
				Concurrency: 2,
				BatchSize:   batchSize,
			}

			got := r.Run(t.Context(), jobs)

			for i := range got {
				got[i].Elapsed = 0
			}
			assert.Equal(t, tc.want(sandbox), got)
			builds := slices.DeleteFunc(slices.Clone(f.calls), func(c call) bool { return c.Name != "go" })
			assert.Len(t, builds, tc.wantBuilds)
			assert.Equal(t, tc.wantProgress, r.Progress())
		})
	}
}

func TestRunnerRunWritesConfig(t *testing.T) {
	t.Parallel()

	sandbox := t.TempDir()
	f := &fakeExec{respond: respond(writeGen, buildOK)}
	r := &Runner{Exec: f.run, Sandbox: Sandbox{Dir: sandbox}, Tool: "/bin/codegen", Timeout: time.Minute, BatchSize: 1}
	jobs := Jobs([]Spec{{Name: "3.0/pets.yml", Path: "/specs/3.0/pets.yml"}}, []Variant{{Name: "chi", Config: "server: {}\n"}})

	r.Run(t.Context(), jobs)

	dir := filepath.Join(sandbox, "specs", "chi", "s3_0_pets")
	data, err := os.ReadFile(filepath.Join(dir, "codegen.yml"))
	require.NoError(t, err)
	assert.Equal(t, "package: s3_0_pets\noutput:\n  file: ./gen.go\nserver: {}\n", string(data))
	assert.Equal(t, []call{
		{Dir: dir, Name: "/bin/codegen", Args: []string{"generate", "-c", "codegen.yml", "/specs/3.0/pets.yml"}},
		{Dir: sandbox, Name: "go", Args: []string{"build", "./specs/chi/s3_0_pets"}},
	}, f.calls)
}

func TestByPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		out  string
		pkgs []string
		want map[string]string
	}{
		{
			name: "Output under each header",
			out:  "# sandbox/specs/a\nerr a1\nerr a2\n# sandbox/specs/b\nerr b\n",
			pkgs: []string{"specs/a", "specs/b"},
			want: map[string]string{"specs/a": "err a1\nerr a2", "specs/b": "err b"},
		},
		{
			name: "Lines before the first header and other packages are left out",
			out:  "go: warning\n# sandbox/specs/x\nerr x\n# sandbox/specs/a\nerr a\n",
			pkgs: []string{"specs/a"},
			want: map[string]string{"specs/a": "err a"},
		},
		{name: "No header", out: "go: cannot find main module\n", pkgs: []string{"specs/a"}, want: map[string]string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, byPackage([]byte(tc.out), tc.pkgs))
		})
	}
}

// respond answers CLI runs with gen and go builds with build.
func respond(gen generator, build builder) func(context.Context, call) ([]byte, error) {
	return func(ctx context.Context, c call) ([]byte, error) {
		if c.Name != "go" {
			spec := path.Base(c.Args[len(c.Args)-1])
			return gen(ctx, c.Dir, strings.TrimSuffix(spec, path.Ext(spec)))
		}
		pkgs := make([]string, 0, len(c.Args)-1)
		for _, a := range c.Args[1:] {
			pkgs = append(pkgs, path.Base(a))
		}
		return build(pkgs)
	}
}

func writeGen(_ context.Context, dir, _ string) ([]byte, error) {
	return nil, os.WriteFile(filepath.Join(dir, "gen.go"), []byte("package p\n\ntype T int\n"), 0o644)
}

func buildOK([]string) ([]byte, error) {
	return nil, nil
}
