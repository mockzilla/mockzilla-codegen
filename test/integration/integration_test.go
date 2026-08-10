// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/itest"
)

const (
	knownFailuresFile = "known-failures.txt"
	cacheFile         = ".integration-cache.json"
	specTimeout       = 5 * time.Minute
	batchSize         = 50
	progressEvery     = 5 * time.Second
)

var variants = []itest.Variant{{Name: "models"}}

// TestIntegration generates every spec in testdata/specs and builds the result. It fails on an
// unlisted failure and on a listed spec that passes now.
func TestIntegration(t *testing.T) {
	t.Parallel()

	require.Empty(t, os.Getenv("FRAMEWORKS"), "FRAMEWORKS needs server generation")
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	named := strings.Fields(os.Getenv("SPEC") + " " + os.Getenv("SPECS"))
	specs, err := itest.Collect(repo, filepath.Join(repo, "testdata", "specs"), named)
	require.NoError(t, err)
	if len(specs) == 0 {
		t.Skip("no specs in testdata/specs")
	}

	knownData, err := os.ReadFile(knownFailuresFile)
	require.NoError(t, err)
	known, err := itest.ParseKnown(knownData)
	require.NoError(t, err)

	concurrency := runtime.GOMAXPROCS(0)
	if s := os.Getenv("INTEGRATION_MAX_CONCURRENCY"); s != "" {
		concurrency, err = strconv.Atoi(s)
		require.NoError(t, err, "INTEGRATION_MAX_CONCURRENCY")
	}

	ctx := t.Context()
	start := time.Now()
	sandbox := itest.Sandbox{Dir: filepath.Join(repo, ".sandbox", "integration"), Repo: repo}
	tool, err := sandbox.BuildTool(ctx, itest.Exec)
	require.NoError(t, err)
	require.NoError(t, sandbox.Setup(ctx, itest.Exec, []string{gomodel.RuntimePath}))
	runtimeFiles, err := filepath.Glob(filepath.Join(repo, "pkg", "runtime", "*.go"))
	require.NoError(t, err)
	toolHash, err := itest.HashFiles(append([]string{tool}, runtimeFiles...)...)
	require.NoError(t, err)

	cachePath := filepath.Join(repo, cacheFile)
	if os.Getenv("CLEAR_CACHE") == "1" {
		require.NoError(t, os.RemoveAll(cachePath))
	}
	cache, err := itest.LoadCache(cachePath, toolHash)
	require.NoError(t, err)
	isCacheRead := len(named) != 1

	var cached []itest.Result
	var jobs []itest.Job
	keys := map[string]string{}
	for _, job := range itest.Jobs(specs, variants) {
		key, keyErr := itest.Key(job)
		require.NoError(t, keyErr)
		keys[job.Package] = key
		if isCacheRead && cache.Passed[key] {
			cached = append(cached, itest.Result{Job: job, IsCached: true})
			continue
		}
		jobs = append(jobs, job)
	}
	fmt.Fprintf(os.Stderr, "%d jobs, %d cached, %d to run on %d workers\n",
		len(jobs)+len(cached), len(cached), len(jobs), concurrency)

	runner := &itest.Runner{
		Exec:        itest.Exec,
		Sandbox:     sandbox,
		Tool:        tool,
		Timeout:     specTimeout,
		Concurrency: concurrency,
		BatchSize:   batchSize,
	}
	results := slices.Concat(cached, runWithProgress(ctx, runner, jobs))

	for _, r := range results {
		if r.Stage == "" && !r.IsCached {
			cache.Passed[keys[r.Job.Package]] = true
		}
	}
	require.NoError(t, cache.Save(cachePath))

	fmt.Fprintf(os.Stderr, "\n%s\ntook %s\n", itest.Report(results, known), time.Since(start).Round(time.Second))
	v := itest.Compare(results, known)
	for _, r := range v.New {
		t.Errorf("%s (%s) failed at %s, not in %s", r.Job.Spec.Name, r.Job.Variant.Name, r.Stage, knownFailuresFile)
	}
	for _, name := range v.Fixed {
		t.Errorf("%s passes now; remove it from %s", name, knownFailuresFile)
	}
}

// runWithProgress runs the jobs and prints the runner's progress while they run.
func runWithProgress(ctx context.Context, runner *itest.Runner, jobs []itest.Job) []itest.Result {
	ticker := time.NewTicker(progressEvery)
	defer ticker.Stop()
	start := time.Now()
	done := make(chan []itest.Result)
	go func() { done <- runner.Run(ctx, jobs) }()

	for {
		select {
		case results := <-done:
			return results
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "%s: %s\n", time.Since(start).Round(time.Second), runner.Progress())
		}
	}
}
