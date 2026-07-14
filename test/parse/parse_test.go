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

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/provider"
	"github.com/mockzilla/codegen/internal/provider/libopenapi"
)

const (
	specsDir          = "../../testdata/specs"
	knownFailuresFile = "known-failures.txt"
	slowestShown      = 10
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
	results := run(specs)

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

func run(specs []string) []result {
	p := libopenapi.New()
	results := make([]result, len(specs))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))

	var wg sync.WaitGroup
	for i, s := range specs {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			results[i] = parseOne(p, s)
		})
	}
	wg.Wait()
	return results
}

func parseOne(p *libopenapi.Provider, spec string) (r result) {
	start := time.Now()
	r.spec = spec
	defer func() { r.elapsed = time.Since(start) }()

	path, err := filepath.Abs(filepath.Join(specsDir, spec))
	if err != nil {
		return fail(r, "read", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fail(r, "read", err)
	}

	ctx := context.Background()
	bundled, err := p.Bundle(ctx, provider.Source{Data: data, Path: path})
	if err != nil {
		return fail(r, "bundle", err)
	}
	_, diags, err := p.Parse(ctx, bundled.Data, provider.ParseOptions{File: spec})
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
