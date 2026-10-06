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
	"maps"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/itest"
	"github.com/mockzilla/mockzilla-codegen/internal/prepare"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/provider/libopenapi"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

const (
	specsDir       = "../../testdata/specs"
	slowestShown   = 10
	preparedSample = 20
)

type result struct {
	spec    string
	stage   string
	err     string
	elapsed time.Duration
	codes   map[string]int
}

// TestParse fails on every spec that does not load.
func TestParse(t *testing.T) {
	t.Parallel()

	results := run(t, collect(t), "spec: {prune: false}")

	failed, passed := 0, 0
	codes := map[string]int{}
	for _, r := range results {
		for code, n := range r.codes {
			codes[code] += n
		}
		if r.stage == "" {
			passed++
			continue
		}
		failed++
		t.Errorf("%s failed at %s: %s", r.spec, r.stage, r.err)
	}

	t.Logf("%d specs: %d passed, %d failed", len(results), passed, failed)
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

	specs := collect(t)
	step := max(len(specs)/preparedSample, 1)
	var sample []itest.Spec
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

// collect returns the specs SPEC and SPECS name, or every spec in testdata/specs, sorted by name.
func collect(t *testing.T) []itest.Spec {
	t.Helper()

	named := strings.Fields(os.Getenv("SPEC") + " " + os.Getenv("SPECS"))
	specs, err := itest.Collect("../..", specsDir, named)
	require.NoError(t, err)
	if len(specs) == 0 {
		t.Skip("no specs in testdata/specs")
	}
	slices.SortFunc(specs, func(a, b itest.Spec) int { return cmp.Compare(a.Name, b.Name) })
	return specs
}

func run(t *testing.T, specs []itest.Spec, cfgSrc string) []result {
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

func parseOne(p *libopenapi.Provider, cfg *config.Config, spec itest.Spec) (r result) {
	start := time.Now()
	r.spec = spec.Name
	defer func() { r.elapsed = time.Since(start) }()

	ctx := context.Background()
	out, err := prepare.Run(ctx, p, prepare.Input{Path: spec.Path, Config: cfg})
	if err != nil {
		return fail(r, "prepare", err)
	}
	_, diags, err := p.Parse(ctx, out.Bytes, provider.ParseOptions{File: spec.Name, Positions: out.Positions})
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
