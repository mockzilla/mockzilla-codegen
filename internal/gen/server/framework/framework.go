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

// Route is one operation on the router. Pattern is the route as the framework writes it.
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

// Handler is the shape of the handlers the adapter gives a framework. Signature follows the
// handler's name: its parameters, and its result when it has one. Prologue is the statement that
// binds w and r, the response writer and the request, when the parameters are not those; Return
// is the statement that leaves the handler early and Epilogue the one that ends it, both empty
// when the handler needs none.
type Handler struct {
	Signature string
	Prologue  string
	Return    string
	Epilogue  string
}

// Framework is one HTTP framework a router is generated for.
type Framework interface {
	Name() string
	Family() Family
	Imports() []gomodel.Import
	// RoutePattern writes an operation's method and OpenAPI path as the router takes them, or
	// fails for a path the router rejects.
	RoutePattern(method, path string) (string, error)
	// Conflicts drops every route the router cannot hold next to an earlier one, with the reason.
	Conflicts(routes []Route) ([]Route, []Conflict)
	// Handler is the shape of the adapter's handlers, with the framework's types written as the
	// file of s spells them.
	Handler(s *gocode.Scope) Handler
	// PathParam writes the expression that reads a path parameter in a handler, where r is the
	// request and c the framework's context, when the handler has one.
	PathParam(s *gocode.Scope, name string) string
	// Templates holds router.tmpl, the template of the router part.
	Templates() fs.FS
}

// HTTPHandler is the handler shape of the NetHTTP family: an http.HandlerFunc.
func HTTPHandler(s *gocode.Scope) Handler {
	pkg := s.Import(gomodel.Import{Path: "net/http"})
	return Handler{Signature: "(w " + pkg + ".ResponseWriter, r *" + pkg + ".Request)", Return: "return"}
}

// Params lists the path parameters of a path, in order.
func Params(path string) []string {
	var out []string
	for _, m := range param.FindAllStringSubmatch(path, -1) {
		out = append(out, m[1])
	}
	return out
}

// Shape is a pattern with its parameter names blanked, so patterns that differ in the names of
// their parameters alone compare equal.
func Shape(pattern string) string {
	return param.ReplaceAllString(pattern, "{}")
}

// ConflictsByShape drops every route that has the method and the shape of an earlier one's path:
// a repeat of it, or one whose path parameters are named otherwise. It serves routers that key
// parameters by position and let a later route replace an earlier one.
func ConflictsByShape(routes []Route) ([]Route, []Conflict) {
	var kept []Route
	var dropped []Conflict
	seen := map[string]Route{}
	for _, r := range routes {
		key := r.Method + " " + Shape(r.Path)
		first, isTaken := seen[key]
		switch {
		case !isTaken:
			seen[key] = r
			kept = append(kept, r)
		case first.Path == r.Path:
			dropped = append(dropped, Conflict{Route: r, Reason: "repeats the route of " + first.Operation})
		default:
			dropped = append(dropped, Conflict{Route: r, Reason: "names its path parameters otherwise than " + first.Operation + " at " + first.Path})
		}
	}
	return kept, dropped
}
