// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

//go:build integration

package integration

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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/itest"
)

const (
	cacheFile        = ".integration-cache.json"
	specTimeout      = 5 * time.Minute
	batchSize        = 50
	progressEvery    = 5 * time.Second
	serverValidation = "  validation:\n    request: true\n    response: true\n"
	modelsValidation = "models:\n  validation:\n    response: true\n"
)

// servers are the frameworks FRAMEWORKS can name, with the modules their code imports.
var servers = map[string][]string{
	"chi":         {"github.com/go-chi/chi/v5"},
	"std-http":    nil,
	"echo":        {"github.com/labstack/echo/v4"},
	"echo-v5":     {"github.com/labstack/echo/v5"},
	"gin":         {"github.com/gin-gonic/gin"},
	"gorilla-mux": {"github.com/gorilla/mux"},
	"fiber":       {"github.com/gofiber/fiber/v3", "github.com/valyala/fasthttp/fasthttpadaptor"},
	"fasthttp":    {"github.com/fasthttp/router", "github.com/valyala/fasthttp/fasthttpadaptor"},
	"hertz":       {"github.com/cloudwego/hertz/pkg/app/server"},
	"beego":       {"github.com/beego/beego/v2/server/web"},
	"goframe":     {"github.com/gogf/gf/v2/net/ghttp"},
	"go-zero":     {"github.com/zeromicro/go-zero/rest"},
	"iris":        {"github.com/kataras/iris/v12"},
	"kratos":      {"github.com/go-kratos/kratos/v2/transport/http"},
}

// clientVariant generates the client with its envelopes and its stream methods, the largest of
// its shapes, and builds one against a base URL.
var clientVariant = itest.Variant{Name: "client", Config: "client:\n  with-response: true\n  streaming: true\n", Init: "%s.NewClient(\"http://localhost\")"}

// mcpVariant generates the MCP tools over the client, builds them and registers them on a server,
// which checks the input schema of every tool; mcpDeps are the modules their code imports.
var (
	mcpVariant = itest.Variant{
		Name:    "mcp",
		Config:  "client: {}\nmcp: {}\n",
		Init:    `c, _ := %[1]s.NewClient("http://localhost"); %[1]s.NewMCPTools(c).Register(mcp.NewServer(&mcp.Implementation{Name: "check"}, nil))`,
		Imports: mcpDeps,
	}
	mcpDeps = []string{"github.com/modelcontextprotocol/go-sdk/mcp"}
)

// splitVariant puts every part of a chi server with scaffolds, the client and the MCP tools in a
// package of its own where Go allows, so the build checks every reference between packages.
var splitVariant = itest.Variant{
	Name: "split",
	Config: "server:\n  framework: chi\n" + serverValidation + "  scaffold:\n" +
		"    service: ./scaffold/service/service.go\n" +
		"    middleware: ./scaffold/middleware/middleware.go\n" +
		"    main: ./cmd/server/main.go\n" +
		"client:\n  with-response: true\n  streaming: true\nmcp: {}\n",
	Files: map[string][]string{
		"./models/gen.go":           {"models"},
		"./server/service/gen.go":   {"server.service"},
		"./server/adapter/gen.go":   {"server.adapter"},
		"./server/router/gen.go":    {"server.router"},
		"./server/errors/gen.go":    {"server.errors"},
		"./client/core/gen.go":      {"client.core", "client.operations"},
		"./client/options/gen.go":   {"client.options"},
		"./client/responses/gen.go": {"client.responses"},
		"./mcp/tools/gen.go":        {"mcp.tools"},
		"./mcp/inputs/gen.go":       {"mcp.inputs"},
	},
}

// TestIntegration generates every spec in testdata/specs with the models variant, one per
// framework FRAMEWORKS names (chi by default, every framework for all), the client variant when
// CLIENT is set, the MCP variant when MCP is set and the split variant when SPLIT is set, then
// builds and tests the result. A spec ROUTER_SPECS names runs on every other framework too, and
// BATCH=i/n keeps the jobs of the i-th of n batches. It fails on every failed job.
func TestIntegration(t *testing.T) {
	t.Parallel()

	variants, deps, err := selectVariants(os.LookupEnv("FRAMEWORKS"))
	require.NoError(t, err)
	if os.Getenv("CLIENT") != "" {
		variants = append(variants, clientVariant)
	}
	if os.Getenv("MCP") != "" {
		variants = append(variants, mcpVariant)
		deps = append(deps, mcpDeps...)
	}
	if os.Getenv("SPLIT") != "" {
		variants = append(variants, splitVariant)
		deps = append(deps, servers["chi"]...)
		deps = append(deps, mcpDeps...)
	}
	repo, err := filepath.Abs("../..")
	require.NoError(t, err)
	named := strings.Fields(os.Getenv("SPEC") + " " + os.Getenv("SPECS"))
	specs, err := itest.Collect(repo, filepath.Join(repo, "testdata", "specs"), named)
	require.NoError(t, err)
	if len(specs) == 0 {
		t.Skip("no specs in testdata/specs")
	}

	all := itest.Jobs(specs, variants)
	if routed := routedSpecs(specs, strings.Fields(os.Getenv("ROUTER_SPECS"))); len(routed) > 0 {
		others, otherDeps := otherServers(variants)
		all = append(all, itest.Jobs(routed, others)...)
		deps = append(deps, otherDeps...)
	}
	all, err = pickBatch(all, os.Getenv("BATCH"))
	require.NoError(t, err)

	slices.Sort(deps)
	deps = slices.Compact(deps)

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
	require.NoError(t, sandbox.Setup(ctx, itest.Exec, deps))
	runtimeFiles, err := goFiles(filepath.Join(repo, "pkg", "runtime"))
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
	for _, job := range all {
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

	fmt.Fprintf(os.Stderr, "\n%s\ntook %s\n", itest.Report(results), time.Since(start).Round(time.Second))
	for _, r := range results {
		if r.Stage != "" {
			t.Errorf("%s (%s) failed at %s", r.Job.Spec.Name, r.Job.Variant.Name, r.Stage)
		}
	}
}

// selectVariants is the models variant and the server variants of the frameworks named, chi when
// none is set, with the modules the sandbox needs.
func selectVariants(frameworks string, isSet bool) ([]itest.Variant, []string, error) {
	names := strings.FieldsFunc(frameworks, func(r rune) bool { return r == ',' || r == ' ' })
	switch {
	case !isSet:
		names = []string{"chi"}
	case frameworks == "all":
		names = slices.Sorted(maps.Keys(servers))
	}
	variants := []itest.Variant{{Name: "models", Config: modelsValidation}}
	deps := []string{gomodel.RuntimePath}
	for _, name := range names {
		serverDeps, ok := servers[name]
		if !ok {
			return nil, nil, fmt.Errorf("FRAMEWORKS names %q, which has no integration variant", name)
		}
		variants = append(variants, serverVariant(name))
		deps = append(deps, serverDeps...)
	}
	return variants, deps, nil
}

// serverVariant generates the server of framework with request and response validation.
func serverVariant(framework string) itest.Variant {
	return itest.Variant{
		Name:   framework,
		Config: "server:\n  framework: " + framework + "\n" + serverValidation,
		Init:   "%s.NewRouter(nil)",
	}
}

// otherServers is a server variant of every framework variants has none of, with its modules.
func otherServers(variants []itest.Variant) ([]itest.Variant, []string) {
	var others []itest.Variant
	var deps []string
	for _, name := range slices.Sorted(maps.Keys(servers)) {
		if !slices.ContainsFunc(variants, func(v itest.Variant) bool { return v.Name == name }) {
			others = append(others, serverVariant(name))
			deps = append(deps, servers[name]...)
		}
	}
	return others, deps
}

// routedSpecs are the specs whose name is in names.
func routedSpecs(specs []itest.Spec, names []string) []itest.Spec {
	var routed []itest.Spec
	for _, s := range specs {
		if slices.Contains(names, s.Name) {
			routed = append(routed, s)
		}
	}
	return routed
}

// pickBatch keeps batch i of n for BATCH=i/n; each job joins the batch with the fewest spec bytes.
func pickBatch(jobs []itest.Job, batch string) ([]itest.Job, error) {
	if batch == "" {
		return jobs, nil
	}
	first, count, _ := strings.Cut(batch, "/")
	i, iErr := strconv.Atoi(first)
	n, nErr := strconv.Atoi(count)
	if iErr != nil || nErr != nil || i < 1 || i > n {
		return nil, fmt.Errorf("BATCH is %q, want i/n with i from 1 to n", batch)
	}

	heaviest := slices.Clone(jobs)
	slices.SortStableFunc(heaviest, func(a, b itest.Job) int {
		return cmp.Compare(b.Spec.Size, a.Spec.Size)
	})
	loads := make([]int64, n)
	var picked []itest.Job
	for _, j := range heaviest {
		lightest := slices.Index(loads, slices.Min(loads))
		loads[lightest] += j.Spec.Size
		if lightest == i-1 {
			picked = append(picked, j)
		}
	}
	return picked, nil
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

// goFiles lists the Go files in dir and the folders below it.
func goFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return err
	})
	return files, err
}
