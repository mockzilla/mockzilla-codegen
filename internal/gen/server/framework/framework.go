// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package framework is what the router of one HTTP framework needs from the generator, and what
// every framework shares.
package framework

import (
	"io/fs"
	"regexp"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

// Family says how a framework's handlers are shaped.
type Family int

const (
	// NetHTTP frameworks take http.HandlerFunc handlers.
	NetHTTP Family = iota
	// Native frameworks take handlers of their own shape.
	Native
)

// param is a path parameter as OpenAPI writes it.
var param = regexp.MustCompile(`\{([^{}]*)\}`)

// Route is one operation on the router. Pattern is the path as the framework writes it.
type Route struct {
	Operation string
	Method    string
	Path      string
	Pattern   string
}

// Conflict is a route the router cannot hold next to an earlier one.
type Conflict struct {
	Route  Route
	Reason string
}

// Framework is one HTTP framework a router is generated for.
type Framework interface {
	Name() string
	Family() Family
	Imports() []gomodel.Import
	// RoutePattern writes an OpenAPI path as the router takes it, or fails for a path the router
	// rejects.
	RoutePattern(path string) (string, error)
	// PathParam writes the expression that reads a path parameter in a handler, where r is the
	// request.
	PathParam(s *gocode.Scope, name string) string
	// Templates holds router.tmpl, the template of the router part.
	Templates() fs.FS
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, which routers key by position.
func Conflicts(routes []Route) ([]Route, []Conflict) {
	var kept []Route
	var dropped []Conflict
	seen := map[string]Route{}
	for _, r := range routes {
		key := r.Method + " " + param.ReplaceAllString(r.Pattern, "{}")
		first, isTaken := seen[key]
		switch {
		case !isTaken:
			seen[key] = r
			kept = append(kept, r)
		case first.Pattern == r.Pattern:
			dropped = append(dropped, Conflict{Route: r, Reason: "repeats the route of " + first.Operation})
		default:
			dropped = append(dropped, Conflict{Route: r, Reason: "names its path parameters otherwise than " + first.Operation + " at " + first.Path})
		}
	}
	return kept, dropped
}

// Params lists the path parameters of a path, in order.
func Params(path string) []string {
	var out []string
	for _, m := range param.FindAllStringSubmatch(path, -1) {
		out = append(out, m[1])
	}
	return out
}
