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

// servers are the server variants FRAMEWORKS can name, with the modules their code imports. The
// init call builds the router, which panics on a route the framework rejects.
var servers = map[string]struct {
	variant itest.Variant
	deps    []string
}{
	"chi": {
		variant: itest.Variant{Name: "chi", Config: "server:\n  framework: chi\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/go-chi/chi/v5"},
	},
	"std-http": {
		variant: itest.Variant{Name: "std-http", Config: "server:\n  framework: std-http\n", Init: "%s.NewRouter(nil)"},
	},
	"echo": {
		variant: itest.Variant{Name: "echo", Config: "server:\n  framework: echo\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/labstack/echo/v4"},
	},
	"echo-v5": {
		variant: itest.Variant{Name: "echo-v5", Config: "server:\n  framework: echo-v5\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/labstack/echo/v5"},
	},
	"gin": {
		variant: itest.Variant{Name: "gin", Config: "server:\n  framework: gin\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/gin-gonic/gin"},
	},
	"gorilla-mux": {
		variant: itest.Variant{Name: "gorilla-mux", Config: "server:\n  framework: gorilla-mux\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/gorilla/mux"},
	},
	"fiber": {
		variant: itest.Variant{Name: "fiber", Config: "server:\n  framework: fiber\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/gofiber/fiber/v3", "github.com/valyala/fasthttp/fasthttpadaptor"},
	},
	"fasthttp": {
		variant: itest.Variant{Name: "fasthttp", Config: "server:\n  framework: fasthttp\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/fasthttp/router", "github.com/valyala/fasthttp/fasthttpadaptor"},
	},
	"hertz": {
		variant: itest.Variant{Name: "hertz", Config: "server:\n  framework: hertz\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/cloudwego/hertz/pkg/app/server"},
	},
	"beego": {
		variant: itest.Variant{Name: "beego", Config: "server:\n  framework: beego\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/beego/beego/v2/server/web"},
	},
	"goframe": {
		variant: itest.Variant{Name: "goframe", Config: "server:\n  framework: goframe\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/gogf/gf/v2/net/ghttp"},
	},
	"go-zero": {
		variant: itest.Variant{Name: "go-zero", Config: "server:\n  framework: go-zero\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/zeromicro/go-zero/rest"},
	},
	"iris": {
		variant: itest.Variant{Name: "iris", Config: "server:\n  framework: iris\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/kataras/iris/v12"},
	},
	"kratos": {
		variant: itest.Variant{Name: "kratos", Config: "server:\n  framework: kratos\n", Init: "%s.NewRouter(nil)"},
		deps:    []string{"github.com/go-kratos/kratos/v2/transport/http"},
	},
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

// TestIntegration generates every spec in testdata/specs with the models variant, one per
// framework FRAMEWORKS names, chi by default, the client variant when CLIENT is set and the MCP
// variant when MCP is set, then builds and tests the result. It fails on an unlisted failure and
// on a listed spec that passes now.
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
	require.NoError(t, sandbox.Setup(ctx, itest.Exec, deps))
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

// selectVariants is the models variant and the server variants of the frameworks named, chi when
// none is set, with the modules the sandbox needs.
func selectVariants(frameworks string, isSet bool) ([]itest.Variant, []string, error) {
	if !isSet {
		frameworks = "chi"
	}
	variants := []itest.Variant{{Name: "models"}}
	deps := []string{gomodel.RuntimePath}
	for _, name := range strings.FieldsFunc(frameworks, func(r rune) bool { return r == ',' || r == ' ' }) {
		s, ok := servers[name]
		if !ok {
			return nil, nil, fmt.Errorf("FRAMEWORKS names %q, which has no integration variant", name)
		}
		variants = append(variants, s.variant)
		deps = append(deps, s.deps...)
	}
	return variants, deps, nil
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
