// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// One spec generated with one config variant, and how it went.

package itest

import (
	"context"
	"os/exec"
	"path"
	"time"
)

// Stages a job can fail at.
const (
	StageGenerate = "generate"
	StageBuild    = "build"
	StageTest     = "test"
)

// waitDelay bounds the wait for output pipes that a killed process's children keep open.
const waitDelay = 10 * time.Second

// Command runs name with args in dir and returns its combined output.
type Command func(ctx context.Context, dir, name string, args ...string) ([]byte, error)

// Variant is one config every spec is generated with. Config is YAML added to the package and
// output keys the runner writes. Files are the entries of output.files, the selectors of the parts
// each file gets. Init, when set, is a call the runner makes on every package once it builds, with
// %s standing for the package, such as %s.NewRouter(nil); a panic fails the job. Imports are the
// other packages Init names.
type Variant struct {
	Name    string
	Config  string
	Files   map[string][]string
	Init    string
	Imports []string
}

// Job is one spec generated with one variant into Package, a folder of the sandbox with forward
// slashes whose last element is the Go package name.
type Job struct {
	Spec    Spec
	Variant Variant
	Package string
}

// Result is how a job went: an empty Stage means it passed. Output is what the failing step
// printed. Lines counts the generated lines.
type Result struct {
	Job      Job
	Stage    string
	Output   string
	Lines    int
	Elapsed  time.Duration
	IsCached bool
}

// Exec runs name as a process in dir.
func Exec(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.WaitDelay = waitDelay
	return cmd.CombinedOutput()
}

// Jobs pairs every spec with every variant, spec by spec.
func Jobs(specs []Spec, variants []Variant) []Job {
	names := make([]string, len(specs))
	for i, s := range specs {
		names[i] = s.Name
	}
	safe := SafeNames(names)

	jobs := make([]Job, 0, len(specs)*len(variants))
	for i, s := range specs {
		for _, v := range variants {
			jobs = append(jobs, Job{Spec: s, Variant: v, Package: path.Join(specsFolder, v.Name, safe[i])})
		}
	}
	return jobs
}
