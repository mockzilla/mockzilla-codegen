// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const configFile = "codegen.yml"

// Runner generates jobs with the CLI at Tool and builds what they generate in the sandbox.
type Runner struct {
	Exec        Command
	Sandbox     Sandbox
	Tool        string
	Timeout     time.Duration
	Concurrency int
	BatchSize   int

	total      atomic.Int64
	generating atomic.Int64
	building   atomic.Int64
	done       atomic.Int64
	failed     atomic.Int64
}

// Run generates up to Concurrency jobs at a time and builds the generated packages in batches of
// BatchSize while generation goes on. Results are in job order.
func (r *Runner) Run(ctx context.Context, jobs []Job) []Result {
	r.total.Add(int64(len(jobs)))
	results := make([]Result, len(jobs))
	generated := make(chan int, len(jobs))

	go func() {
		sem := make(chan struct{}, max(r.Concurrency, 1))
		var wg sync.WaitGroup
		for i := range jobs {
			sem <- struct{}{}
			wg.Go(func() {
				defer func() { <-sem }()
				results[i] = r.generate(ctx, jobs[i])
				if results[i].Stage == "" {
					generated <- i
				}
			})
		}
		wg.Wait()
		close(generated)
	}()

	var batch []int
	for i := range generated {
		batch = append(batch, i)
		if len(batch) >= r.BatchSize {
			r.build(ctx, batch, results)
			batch = nil
		}
	}
	if len(batch) > 0 {
		r.build(ctx, batch, results)
	}
	return results
}

// Progress is one line on how far Run is; safe to call while it runs.
func (r *Runner) Progress() string {
	return fmt.Sprintf("%d/%d done, %d failed, %d generating, %d building",
		r.done.Load(), r.total.Load(), r.failed.Load(), r.generating.Load(), r.building.Load())
}

func (r *Runner) generate(ctx context.Context, job Job) Result {
	r.generating.Add(1)
	defer r.generating.Add(-1)

	start := time.Now()
	dir := filepath.Join(r.Sandbox.Dir, filepath.FromSlash(job.Package))
	out, err := r.runTool(ctx, dir, job)
	res := Result{Job: job, Elapsed: time.Since(start)}
	if err == nil {
		res.Lines, err = countLines(dir)
	}
	if err != nil {
		res.Stage, res.Output = StageGenerate, joinOutput(out, err)
		r.done.Add(1)
		r.failed.Add(1)
	}
	return res
}

func (r *Runner) runTool(ctx context.Context, dir string, job Job) ([]byte, error) {
	cfg := "package: " + path.Base(job.Package) + "\noutput:\n  file: ./gen.go\n" + job.Variant.Config
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte(cfg), 0o644); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	out, err := r.Exec(ctx, dir, r.Tool, "generate", "-c", configFile, job.Spec.Path)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("%w after %s", ErrTimeout, r.Timeout)
	}
	return out, err
}

func (r *Runner) build(ctx context.Context, batch []int, results []Result) {
	n := int64(len(batch))
	r.building.Add(n)
	defer func() {
		r.building.Add(-n)
		r.done.Add(n)
	}()

	pkgs := make([]string, len(batch))
	for k, i := range batch {
		pkgs[k] = results[i].Job.Package
	}
	failures := r.buildPackages(ctx, pkgs)
	for k, i := range batch {
		if out, isFailed := failures[pkgs[k]]; isFailed {
			results[i].Stage, results[i].Output = StageBuild, out
			r.failed.Add(1)
		}
	}
}

// buildPackages returns the output of each of pkgs that fails to build. Packages a failed build
// does not name are built again, one at a time when it names none.
func (r *Runner) buildPackages(ctx context.Context, pkgs []string) map[string]string {
	args := []string{"build"}
	for _, p := range pkgs {
		args = append(args, "./"+p)
	}
	out, err := r.Exec(ctx, r.Sandbox.Dir, "go", args...)
	if err == nil {
		return nil
	}

	failures := byPackage(out, pkgs)
	switch {
	case len(failures) > 0:
		var rest []string
		for _, p := range pkgs {
			if _, isFailed := failures[p]; !isFailed {
				rest = append(rest, p)
			}
		}
		if len(rest) > 0 {
			maps.Copy(failures, r.buildPackages(ctx, rest))
		}
	case len(pkgs) == 1:
		failures[pkgs[0]] = joinOutput(out, err)
	default:
		for _, p := range pkgs {
			maps.Copy(failures, r.buildPackages(ctx, []string{p}))
		}
	}
	return failures
}

// byPackage splits go build output at its "# <import path>" lines and returns the part under
// each of pkgs.
func byPackage(out []byte, pkgs []string) map[string]string {
	want := make(map[string]bool, len(pkgs))
	for _, p := range pkgs {
		want[p] = true
	}

	sections := map[string]*strings.Builder{}
	var cur *strings.Builder
	for line := range strings.Lines(string(out)) {
		if p, isHeader := strings.CutPrefix(line, "# "+sandboxModule+"/"); isHeader {
			cur = nil
			if p = strings.TrimSpace(p); want[p] {
				cur = &strings.Builder{}
				sections[p] = cur
			}
			continue
		}
		if cur != nil {
			cur.WriteString(line)
		}
	}

	failures := make(map[string]string, len(sections))
	for p, b := range sections {
		failures[p] = strings.TrimSpace(b.String())
	}
	return failures
}

func countLines(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	lines := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".go" {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			return 0, readErr
		}
		lines += bytes.Count(data, []byte("\n"))
	}
	return lines, nil
}

func joinOutput(out []byte, err error) string {
	return strings.TrimSpace(strings.TrimSpace(string(out)) + "\n" + err.Error())
}
