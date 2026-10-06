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
	"regexp"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/fasthttp/router"

//go:embed templates/*.tmpl
var templates embed.FS

// braced is a parameter of a pattern.
var braced = regexp.MustCompile(`\{[^{}]*\}`)

var _ framework.Framework = Framework{}

// Framework is the fasthttp router.
type Framework struct{}

func (Framework) Name() string {
	return "fasthttp"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{
		{Path: importPath},
		{Path: "github.com/valyala/fasthttp"},
		{Path: "github.com/valyala/fasthttp/fasthttpadaptor"},
		{Path: "net/http"},
	}
}

// RoutePattern keeps {name} parameters and quotes a literal after one, read as a regexp otherwise.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	if strings.Contains(path, "}{") {
		return "", fmt.Errorf("%w: two parameters must have a character between them", framework.ErrPattern)
	}
	out, err := framework.Brace(path, framework.Unmarked, wildcard)
	if err != nil {
		return "", err
	}
	return quoted(out), nil
}

// Conflicts drops every route the router panics on next to an earlier one of its method.
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
	return gocode.Call(gocode.Selector("r", "PathValue"), gocode.Quote(framework.Unmarked(name)))
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
		i := strings.LastIndex(k.Pattern, "/")
		parent, isCatchAll := k.Pattern[:i], strings.HasSuffix(k.Pattern[i:], ":*}")
		switch {
		case k.Path == r.Path:
			return "repeats the route of " + k.Operation
		case strings.TrimSuffix(framework.Shape(k.Pattern), "/") == strings.TrimSuffix(framework.Shape(r.Pattern), "/"), isCatchAll && r.Pattern == parent:
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

// quoted writes each segment of pattern with the literals after its first parameter quoted.
func quoted(pattern string) string {
	segments := strings.Split(pattern, "/")
	for i, seg := range segments {
		params := braced.FindAllStringIndex(seg, -1)
		if params == nil {
			continue
		}
		var b strings.Builder
		b.WriteString(seg[:params[0][0]])
		for j, p := range params {
			end := len(seg)
			if j+1 < len(params) {
				end = params[j+1][0]
			}
			b.WriteString(seg[p[0]:p[1]])
			b.WriteString(regexp.QuoteMeta(seg[p[1]:end]))
		}
		segments[i] = b.String()
	}
	return strings.Join(segments, "/")
}

// wildcard is the catch-all a trailing /* becomes.
func wildcard(name string) string {
	return "{" + name + ":*}"
}
