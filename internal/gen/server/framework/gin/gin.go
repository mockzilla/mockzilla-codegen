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

// pattern writes routes as gin takes them: a literal colon or star cannot be escaped, and the
// wildcard needs a name.
var pattern = framework.Colon{Literal: framework.Rejecting(":*"), Name: framework.Same, Wildcard: "*rest", IsPrefixAllowed: true}

var _ framework.Framework = Framework{}

// Framework is the gin router.
type Framework struct{}

func (Framework) Name() string {
	return "gin"
}

func (Framework) Family() framework.Family {
	return framework.NetHTTP
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}, {Path: "net/http"}}
}

// RoutePattern writes each parameter as :name and a trailing /* as /*rest, the catch-all gin
// asks a name for. A parameter runs to the end of its segment on gin, so a path fails when one
// has a suffix or shares a segment with another, and a literal colon or star fails since gin
// reads them as the start of a parameter; a path also fails without a leading slash, with an
// unclosed brace, a parameter without a name or named twice, and a wildcard that is not a
// segment of its own, last.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return pattern.Pattern(path)
}

// Conflicts drops every route gin panics on next to an earlier one of its method: a repeat of
// it, a route that names the parameter at some position otherwise, and a route that meets the
// catch-all of an earlier one, or brings one where the earlier route has anything else, since
// gin lets nothing sit next to a catch-all.
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

// conflict is why gin cannot hold r next to the kept routes, or empty when it can. The first
// segment where two routes of one method differ decides: two parameters after one literal
// prefix, or a catch-all on either side, is a conflict.
func conflict(r framework.Route, kept []framework.Route) string {
	for _, k := range kept {
		if k.Method != r.Method {
			continue
		}
		if k.Pattern == r.Pattern {
			return "repeats the route of " + k.Operation
		}
		a, b := strings.Split(k.Pattern[1:], "/"), strings.Split(r.Pattern[1:], "/")
		for i := range min(len(a), len(b)) {
			if a[i] == b[i] {
				continue
			}
			prefixA, isParamA := parameter(a[i])
			prefixB, isParamB := parameter(b[i])
			switch {
			case a[i] == pattern.Wildcard || b[i] == pattern.Wildcard:
				return "cannot sit next to the wildcard of " + k.Operation + " at " + k.Path
			case isParamA && isParamB && prefixA == prefixB:
				return "names its path parameters otherwise than " + k.Operation + " at " + k.Path
			}
			break
		}
	}
	return ""
}

// parameter takes a segment of a pattern apart: the literal before its parameter, and whether it
// has one.
func parameter(segment string) (string, bool) {
	prefix, _, isParam := strings.Cut(segment, ":")
	return prefix, isParam
}
