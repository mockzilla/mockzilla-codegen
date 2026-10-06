// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package gin is the router for github.com/gin-gonic/gin. The handlers stay http.HandlerFuncs,
// served through gin's context with the path parameters on the request.
package gin

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/gin-gonic/gin"

//go:embed templates/*.tmpl
var templates embed.FS

// pattern writes routes as gin takes them: a colon escaped, a star rejected, the wildcard named.
var pattern = framework.Colon{Literal: literal, Name: framework.Unmarked, Wildcard: "*", IsWildcardNamed: true, IsPrefixAllowed: true}

var _ framework.Framework = Framework{}

// Framework is the gin router.
type Framework struct{}

func (Framework) Name() string {
	return "gin"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}, {Path: "net/http"}}
}

// RoutePattern writes each parameter as :name and a trailing /* as /*rest; the path must be clean.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	if err := framework.CheckClean(path); err != nil {
		return "", err
	}
	return pattern.Pattern(path)
}

// Conflicts names each parameter as earlier routes do at its position and drops what gin panics on.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	var kept []framework.Route
	var dropped []framework.Conflict
	for _, r := range routes {
		r.Pattern = rename(r, kept)
		if reason := conflict(r, kept); reason != "" {
			dropped = append(dropped, framework.Conflict{Route: r, Reason: reason})
			continue
		}
		r.Names = names(r)
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

// rename gives each parameter of r the name a kept route of its method has at that position.
func rename(r framework.Route, kept []framework.Route) string {
	b := strings.Split(r.Pattern[1:], "/")
	for _, k := range kept {
		if k.Method != r.Method {
			continue
		}
		a := strings.Split(k.Pattern[1:], "/")
		for i := range min(len(a), len(b)) {
			if a[i] == b[i] {
				continue
			}
			prefixA, isParamA := parameter(a[i])
			prefixB, isParamB := parameter(b[i])
			if !isParamA || !isParamB || prefixA != prefixB {
				break
			}
			b[i] = a[i]
		}
	}
	return "/" + strings.Join(b, "/")
}

// conflict is why gin cannot hold r next to the kept routes, or empty when it can.
func conflict(r framework.Route, kept []framework.Route) string {
	for _, k := range kept {
		if k.Method != r.Method {
			continue
		}
		switch {
		case k.Path == r.Path:
			return "repeats the route of " + k.Operation
		case k.Pattern == r.Pattern:
			return "matches the same requests as " + k.Operation + " at " + k.Path
		}
		a, b := strings.Split(k.Pattern[1:], "/"), strings.Split(r.Pattern[1:], "/")
		for i := range min(len(a), len(b)) {
			if a[i] == b[i] {
				continue
			}
			switch {
			case strings.HasPrefix(a[i], "*"):
				return "cannot sit next to the wildcard of " + k.Operation + " at " + k.Path
			case strings.HasPrefix(b[i], "*"):
				return "brings a wildcard next to " + k.Operation + " at " + k.Path
			}
			break
		}
	}
	return ""
}

// names are the spec's names of the path values of r, the wildcard's last, in gin's order.
func names(r framework.Route) []string {
	out := framework.Params(r.Path)
	if i := strings.LastIndex(r.Pattern, "/*"); i >= 0 {
		out = append(out, r.Pattern[i+2:])
	}
	return out
}

// literal writes a literal as gin takes it: a colon escaped, a star rejected.
func literal(s string) (string, error) {
	if _, err := framework.Rejecting("*")(s); err != nil {
		return "", err
	}
	return framework.Escaping(":")(s)
}

// parameter takes a segment of a pattern apart: the literal before its parameter, and whether it
// has one. An escaped colon is part of the literal.
func parameter(segment string) (string, bool) {
	for i := 0; i < len(segment); i++ {
		switch segment[i] {
		case '\\':
			i++
		case ':':
			return segment[:i], true
		}
	}
	return segment, false
}
