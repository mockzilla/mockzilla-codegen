// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package fasthttp is the router for github.com/fasthttp/router over github.com/valyala/fasthttp.
// The handlers stay http.HandlerFuncs, served through fasthttp's adaptor with the path
// parameters on the request, and the server is served in fasthttp's own way.
package fasthttp

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const (
	importPath = "github.com/fasthttp/router"
	// wildcard is the catch-all a trailing /* becomes.
	wildcard = "{rest:*}"
)

//go:embed *.tmpl
var templates embed.FS

var _ framework.Framework = Framework{}

// Framework is the fasthttp router.
type Framework struct{}

func (Framework) Name() string {
	return "fasthttp"
}

func (Framework) Family() framework.Family {
	return framework.NetHTTP
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{
		{Path: importPath},
		{Path: "github.com/valyala/fasthttp"},
		{Path: "github.com/valyala/fasthttp/fasthttpadaptor"},
		{Path: "net/http"},
	}
}

// RoutePattern keeps the path as it is, since the router writes parameters as {name} too, with a
// trailing /* as the catch-all /{rest:*}. It fails on what the router panics on or misreads: a
// path without a leading slash, an unclosed brace, a wildcard that is not a segment of its own,
// last, two parameters with nothing between them, a parameter without a name, one whose name
// holds a colon, which starts a regular expression, and a parameter named twice.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	if strings.Contains(path, "}{") {
		return "", fmt.Errorf("%w: two parameters must have a character between them", framework.ErrPattern)
	}
	return framework.Brace(path, wildcard)
}

// Conflicts drops every route the router panics on next to an earlier one of its method: a
// route that matches the same requests, which the trailing slash does not tell apart, the parent
// of an earlier catch-all, which the router registers with it, and a route that differs from an
// earlier one in one segment alone where both have a parameter after one literal prefix, since
// the router keys such parameters by position.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	var kept []framework.Route
	var dropped []framework.Conflict
	for _, r := range routes {
		if reason := conflict(r, kept); reason != "" {
			dropped = append(dropped, framework.Conflict{Route: r, Reason: reason})
			continue
		}
		kept = append(kept, r)
	}
	return kept, dropped
}

func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.HTTPHandler(s)
}

func (Framework) PathParam(_ *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector("r", "PathValue"), gocode.Quote(name))
}

func (Framework) Templates() fs.FS {
	return templates
}

// conflict is why the router cannot hold r next to the kept routes, or empty when it can.
func conflict(r framework.Route, kept []framework.Route) string {
	for _, k := range kept {
		if k.Method != r.Method {
			continue
		}
		parent, isCatchAll := strings.CutSuffix(k.Pattern, "/"+wildcard)
		switch {
		case k.Path == r.Path:
			return "repeats the route of " + k.Operation
		case strings.TrimSuffix(k.Pattern, "/") == strings.TrimSuffix(r.Pattern, "/"), isCatchAll && r.Pattern == parent:
			return "matches the same requests as " + k.Operation + " at " + k.Path
		case clashes(k.Pattern, r.Pattern):
			return "clashes with the parameter of " + k.Operation + " at " + k.Path
		}
	}
	return ""
}

// clashes reports whether two patterns differ in one segment alone, where both hold a parameter
// after the same literal prefix.
func clashes(p, q string) bool {
	a, b := strings.Split(p, "/"), strings.Split(q, "/")
	if len(a) != len(b) {
		return false
	}
	differing := -1
	for i := range a {
		if a[i] == b[i] {
			continue
		}
		if differing >= 0 {
			return false
		}
		differing = i
	}
	if differing < 0 {
		return false
	}
	prefixA, _, isParamA := strings.Cut(a[differing], "{")
	prefixB, _, isParamB := strings.Cut(b[differing], "{")
	return isParamA && isParamB && prefixA == prefixB
}
