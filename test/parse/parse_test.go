// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

//go:build parse

package parse_test

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/prepare"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/provider/libopenapi"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

const (
	specsDir          = "../../testdata/specs"
	knownFailuresFile = "known-failures.txt"
	slowestShown      = 10
	preparedSample    = 20
)

type result struct {
	spec    string
	stage   string
	err     string
	elapsed time.Duration
	codes   map[string]int
}

// TestParse fails on an unlisted failure and on a listed spec that now passes.
func TestParse(t *testing.T) {
	t.Parallel()

	specs := collect(t)
	known := loadKnown(t)
	results := run(t, specs, "spec: {prune: false}")

	failed, passed := 0, 0
	codes := map[string]int{}
	for _, r := range results {
		for code, n := range r.codes {
			codes[code] += n
		}
		_, isKnown := known[r.spec]
		switch {
		case r.stage == "" && isKnown:
			t.Errorf("%s passes now; remove it from %s", r.spec, knownFailuresFile)
		case r.stage == "":
			passed++
		case !isKnown:
			failed++
			t.Errorf("%s failed at %s: %s", r.spec, r.stage, r.err)
		default:
			failed++
		}
	}

	t.Logf("%d specs: %d passed, %d failed (%d known)", len(results), passed, failed, len(known))
	for _, code := range slices.Sorted(maps.Keys(codes)) {
		t.Logf("diagnostics %s: %d", code, codes[code])
	}
	slices.SortFunc(results, func(a, b result) int { return cmp.Compare(b.elapsed, a.elapsed) })
	for _, r := range results[:min(slowestShown, len(results))] {
		t.Logf("slow %s: %s", r.spec, r.elapsed.Round(time.Millisecond))
	}
}

// TestPrepared parses a sample of specs after the full Prepare, pruning on: what Prepare writes
// must load again.
func TestPrepared(t *testing.T) {
	t.Parallel()

	known := loadKnown(t)
	var specs []string
	for _, s := range collect(t) {
		if _, isKnown := known[s]; !isKnown {
			specs = append(specs, s)
		}
	}

	step := max(len(specs)/preparedSample, 1)
	var sample []string
	for i := 0; i < len(specs) && len(sample) < preparedSample; i += step {
		sample = append(sample, specs[i])
	}

	for _, r := range run(t, sample, "spec: {prune: true, simplify: {unions: true}}") {
		if r.stage != "" {
			t.Errorf("%s failed at %s: %s", r.spec, r.stage, r.err)
		}
	}
	t.Logf("%d prepared specs parsed", len(sample))
}

func collect(t *testing.T) []string {
	t.Helper()

	var named []string
	if s := os.Getenv("SPEC"); s != "" {
		named = append(named, s)
	}
	named = append(named, strings.Fields(os.Getenv("SPECS"))...)
	if len(named) > 0 {
		return named
	}

	if _, err := os.Stat(specsDir); err != nil {
		t.Skip("no specs in testdata/specs")
	}
	var specs []string
	err := filepath.WalkDir(specsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := filepath.Ext(path); ext == ".yml" || ext == ".yaml" || ext == ".json" {
			rel, _ := filepath.Rel(specsDir, path)
			specs = append(specs, filepath.ToSlash(rel))
		}
		return nil
	})
	require.NoError(t, err)
	slices.Sort(specs)
	return specs
}

func loadKnown(t *testing.T) map[string]string {
	t.Helper()

	data, err := os.ReadFile(knownFailuresFile)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)

	known := map[string]string{}
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		spec, reason, _ := strings.Cut(line, " ")
		known[spec] = strings.TrimSpace(reason)
	}
	return known
}

func run(t *testing.T, specs []string, cfgSrc string) []result {
	t.Helper()

	cfg, err := config.Parse([]byte(cfgSrc), "")
	require.NoError(t, err)
	p := libopenapi.New()
	results := make([]result, len(specs))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))

	var wg sync.WaitGroup
	for i, s := range specs {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			results[i] = parseOne(p, cfg, s)
		})
	}
	wg.Wait()
	return results
}

func parseOne(p *libopenapi.Provider, cfg *config.Config, spec string) (r result) {
	start := time.Now()
	r.spec = spec
	defer func() { r.elapsed = time.Since(start) }()

	ctx := context.Background()
	out, err := prepare.Run(ctx, p, prepare.Input{Path: filepath.Join(specsDir, spec), Config: cfg})
	if err != nil {
		return fail(r, "prepare", err)
	}
	_, diags, err := p.Parse(ctx, out.Bytes, provider.ParseOptions{File: spec, Positions: out.Positions})
	if err != nil {
		return fail(r, "parse", err)
	}

	r.codes = countCodes(diags)
	return r
}

func fail(r result, stage string, err error) result {
	msg, _, _ := strings.Cut(err.Error(), "\n")
	r.stage, r.err = stage, msg
	return r
}

func countCodes(diags []diag.Diagnostic) map[string]int {
	out := map[string]int{}
	for _, d := range diags {
		out[fmt.Sprintf("%s/%s", d.Severity, d.Code)]++
	}
	return out
}
