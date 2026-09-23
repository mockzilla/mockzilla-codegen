// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package framework is what the router of one HTTP framework needs from the generator, and what
// every framework shares.
package framework

import (
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"unicode"

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

// param is a path parameter as OpenAPI writes it; paramSegment is a segment that ends in one
// parameter, with a literal prefix or without.
var (
	param        = regexp.MustCompile(`\{([^{}]*)\}`)
	paramSegment = regexp.MustCompile(`^[^{}]*\{([^{}]*)\}$`)
)

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

// Colon writes routes for a router that takes parameters as :name, running to the end of their
// segment. Literal writes a literal piece of the path, or fails for one the router cannot hold;
// Name writes the name of a parameter; Wildcard is what a trailing /* becomes. A parameter may
// follow a literal prefix in its segment when IsPrefixAllowed is set.
type Colon struct {
	Literal         func(s string) (string, error)
	Name            func(name string) string
	Wildcard        string
	IsPrefixAllowed bool
}

// Pattern writes path as the router takes it. Besides what Check fails on, it fails on a
// parameter with a suffix or one that shares its segment with another, a parameter with a prefix
// unless IsPrefixAllowed, a parameter without a name or named twice, a * that is not a segment of
// its own, and a literal Literal rejects.
func (c Colon) Pattern(path string) (string, error) {
	if err := Check(path); err != nil {
		return "", err
	}

	segments := strings.Split(path[1:], "/")
	var names []string
	for i, seg := range segments {
		if seg == "*" {
			segments[i] = c.Wildcard
			continue
		}
		if !strings.ContainsAny(seg, "{}") {
			lit, err := c.Literal(seg)
			if err != nil {
				return "", err
			}
			segments[i] = lit
			continue
		}
		m := paramSegment.FindStringSubmatch(seg)
		if m == nil {
			return "", fmt.Errorf("%w: a parameter must end its segment, unlike %s", ErrPattern, seg)
		}
		prefix := strings.TrimSuffix(seg, "{"+m[1]+"}")
		switch {
		case prefix != "" && !c.IsPrefixAllowed:
			return "", fmt.Errorf("%w: a parameter must fill its segment, unlike %s", ErrPattern, seg)
		case m[1] == "":
			return "", fmt.Errorf("%w: a parameter has no name", ErrPattern)
		case slices.Contains(names, m[1]):
			return "", fmt.Errorf("%w: parameter %q is named twice", ErrPattern, m[1])
		}
		names = append(names, m[1])
		lit, err := c.Literal(prefix)
		if err != nil {
			return "", err
		}
		segments[i] = lit + ":" + c.Name(m[1])
	}
	return "/" + strings.Join(segments, "/"), nil
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
	return ConflictsByKey(routes, func(r Route) string { return r.Method + " " + Shape(r.Path) })
}

// ConflictsByKey drops every route that has the key of an earlier one, with the reason. Routes
// of one key match the same requests on the router, so the later one would replace or shadow the
// earlier one without a word.
func ConflictsByKey(routes []Route, key func(Route) string) ([]Route, []Conflict) {
	var kept []Route
	var dropped []Conflict
	seen := map[string]Route{}
	for _, r := range routes {
		k := key(r)
		first, isTaken := seen[k]
		switch {
		case !isTaken:
			seen[k] = r
			kept = append(kept, r)
		case first.Path == r.Path:
			dropped = append(dropped, Conflict{Route: r, Reason: "repeats the route of " + first.Operation})
		case Shape(first.Path) == Shape(r.Path):
			dropped = append(dropped, Conflict{Route: r, Reason: "names its path parameters otherwise than " + first.Operation + " at " + first.Path})
		default:
			dropped = append(dropped, Conflict{Route: r, Reason: "matches the same requests as " + first.Operation + " at " + first.Path})
		}
	}
	return kept, dropped
}

// StaticFirst orders routes for a router that takes the first route that matches: at each
// segment a literal comes before a parameter and a parameter before a wildcard, and routes that
// tie keep their order, so no route shadows a more specific one after it.
func StaticFirst(routes []Route) []Route {
	out := slices.Clone(routes)
	slices.SortStableFunc(out, func(a, b Route) int { return slices.Compare(ranks(a.Path), ranks(b.Path)) })
	return out
}

// Identifier is name as a router that takes Go identifiers holds it: any other character becomes
// an underscore, and a leading digit gets one in front.
func Identifier(name string) string {
	var b strings.Builder
	for i, c := range name {
		switch {
		case unicode.IsLetter(c) || c == '_' || (i > 0 && unicode.IsDigit(c)):
			b.WriteRune(c)
		case unicode.IsDigit(c):
			b.WriteRune('_')
			b.WriteRune(c)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// Check fails on what no router takes: a path without a leading slash, a { without its } and a *
// that is not last.
func Check(path string) error {
	switch {
	case !strings.HasPrefix(path, "/"):
		return fmt.Errorf("%w: it must begin with /", ErrPattern)
	case strings.Count(path, "{") != strings.Count(path, "}"):
		return fmt.Errorf("%w: a { has no }", ErrPattern)
	}
	if i := strings.Index(path, "*"); i >= 0 && i != len(path)-1 {
		return fmt.Errorf("%w: * must be last", ErrPattern)
	}
	return nil
}

// Brace writes path for a router that takes parameters as {name} too, with a trailing /* as
// wildcard. Besides what Check fails on, it fails on a * that is not a segment of its own, a
// parameter without a name, one whose name holds a colon, which such routers read as the start
// of a pattern, and a parameter named twice.
func Brace(path, wildcard string) (string, error) {
	if err := Check(path); err != nil {
		return "", err
	}
	names := Params(path)
	for i, name := range names {
		switch {
		case name == "":
			return "", fmt.Errorf("%w: a parameter has no name", ErrPattern)
		case strings.Contains(name, ":"):
			return "", fmt.Errorf("%w: parameter %q holds a colon", ErrPattern, name)
		case slices.Contains(names[:i], name):
			return "", fmt.Errorf("%w: parameter %q is named twice", ErrPattern, name)
		}
	}
	switch rest, isWildcard := strings.CutSuffix(path, "/*"); {
	case isWildcard:
		return rest + "/" + wildcard, nil
	case strings.HasSuffix(path, "*"):
		return "", fmt.Errorf("%w: * must be a segment of its own", ErrPattern)
	}
	return path, nil
}

// Escaping writes a literal with each of the characters in special escaped by a backslash, as
// routers that read them as the start of a parameter take a literal one.
func Escaping(special string) func(string) (string, error) {
	return func(s string) (string, error) {
		var b strings.Builder
		for _, r := range s {
			if strings.ContainsRune(special, r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		return b.String(), nil
	}
}

// Rejecting writes a literal as it is, or fails when it holds one of the characters in special,
// which the router reads as the start of a parameter and cannot escape.
func Rejecting(special string) func(string) (string, error) {
	return func(s string) (string, error) {
		if i := strings.IndexAny(s, special); i >= 0 {
			return "", fmt.Errorf("%w: %s is read as the start of a parameter in %s", ErrPattern, s[i:i+1], s)
		}
		return s, nil
	}
}

// Same writes a name as it is.
func Same(name string) string {
	return name
}

// ranks is how general each segment of path is: 0 for a literal, 1 for a parameter, 2 for *.
func ranks(path string) []int {
	var out []int
	for _, seg := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		switch {
		case seg == "*":
			out = append(out, 2)
		case strings.Contains(seg, "{"):
			out = append(out, 1)
		default:
			out = append(out, 0)
		}
	}
	return out
}
