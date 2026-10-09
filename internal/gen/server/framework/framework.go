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

// param is a path parameter as OpenAPI writes it; paramSegment is a segment that ends in one
// parameter, with a literal prefix or without.
var (
	param        = regexp.MustCompile(`\{([^{}]*)\}`)
	paramSegment = regexp.MustCompile(`^[^{}]*\{([^{}]*)\}$`)
)

// methods are the HTTP methods a router that has one function per method registers.
var methods = []string{"GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE"}

// Route is one operation on the router. Pattern is the route as the framework writes it.
// Names, when set, name its path values in the order the router holds them.
type Route struct {
	Operation string
	Method    string
	Path      string
	Pattern   string
	Names     []string
}

// Conflict is a route the router cannot hold next to an earlier one.
type Conflict struct {
	Route  Route
	Reason string
}

// Handler is the shape of the handlers the adapter gives a framework. Signature follows the
// handler's name: its parameters, and its result when it has one. Writer and Request are the
// response writer and the request the handler has; Epilogue is the statement that ends it, empty
// when it needs none. ServeSignature is the shape of the method that serves an operation inside
// the operation middleware, which takes w and r. Context is the framework's context, when the
// handler has one; ContextServeSignature then takes it before w and r, for an operation that
// reads its path parameters from it.
type Handler struct {
	Signature             string
	Writer                string
	Request               string
	Epilogue              string
	ServeSignature        string
	Context               string
	ContextServeSignature string
}

// Framework is one HTTP framework a router is generated for.
type Framework interface {
	Name() string
	Imports() []gomodel.Import
	// RoutePattern writes an operation's method and OpenAPI path as the router takes them, or
	// fails for a method or a path the router rejects.
	RoutePattern(method, path string) (string, error)
	// Conflicts drops every route the router cannot hold next to an earlier one, with the reason.
	Conflicts(routes []Route) ([]Route, []Conflict)
	// Handler is the shape of the adapter's handlers, with the framework's types written as the
	// file of s spells them.
	Handler(s *gocode.Scope) Handler
	// PathParam writes the expression that reads a path parameter in a handler, where r is the
	// request and c the framework's context, when the handler has one.
	PathParam(s *gocode.Scope, name string) string
	// Templates holds templates/router.tmpl, the template of the router part.
	Templates() fs.FS
}

// Colon writes routes for a router that takes parameters as :name, running to the end of their
// segment. Literal writes a literal piece of the path, or fails for one the router cannot hold;
// Name writes the name of a parameter; Wildcard is what a trailing /* becomes. A parameter may
// follow a literal prefix in its segment when IsPrefixAllowed is set. IsWildcardNamed adds a free
// name to the wildcard.
type Colon struct {
	Literal         func(s string) (string, error)
	Name            func(name string) string
	Wildcard        string
	IsWildcardNamed bool
	IsPrefixAllowed bool
}

// Pattern writes path as the router takes it. Besides what Check fails on, it fails on a
// parameter with a suffix or one that shares its segment with another, a parameter with a prefix
// unless IsPrefixAllowed, a parameter without a name or named twice, a * that is not a segment of
// its own, and a literal Literal rejects. It also fails on two names Name makes one.
func (c Colon) Pattern(path string) (string, error) {
	if err := Check(path); err != nil {
		return "", err
	}

	segments := strings.Split(path[1:], "/")
	var names, given []string
	for i, seg := range segments {
		if seg == "*" {
			segments[i] = c.Wildcard
			if c.IsWildcardNamed {
				segments[i] += RestName(given)
			}
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
		name := c.Name(m[1])
		switch {
		case prefix != "" && !c.IsPrefixAllowed:
			return "", fmt.Errorf("%w: a parameter must fill its segment, unlike %s", ErrPattern, seg)
		case m[1] == "":
			return "", fmt.Errorf("%w: a parameter has no name", ErrPattern)
		}
		if err := checkName(names, m[1], given, name); err != nil {
			return "", err
		}
		names, given = append(names, m[1]), append(given, name)
		lit, err := c.Literal(prefix)
		if err != nil {
			return "", err
		}
		segments[i] = lit + ":" + name
	}
	return "/" + strings.Join(segments, "/"), nil
}

// HTTPHandler is the handler shape of an http.HandlerFunc.
func HTTPHandler(s *gocode.Scope) Handler {
	signature := gocode.Signature(httpParams(s), "")
	return Handler{Signature: signature, Writer: "w", Request: "r", ServeSignature: signature}
}

// ContextHandler is the shape of a handler that takes the router's context and returns nil.
func ContextHandler(s *gocode.Scope, ctxType string) Handler {
	c := gocode.Param("c", ctxType)
	return Handler{
		Signature:             gocode.Signature([]string{c}, "error"),
		Writer:                gocode.Call(gocode.Selector("c", "Response")),
		Request:               gocode.Call(gocode.Selector("c", "Request")),
		Epilogue:              gocode.Return("nil"),
		ServeSignature:        gocode.Signature(httpParams(s), ""),
		Context:               "c",
		ContextServeSignature: gocode.Signature(append([]string{c}, httpParams(s)...), ""),
	}
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
// tie keep their order, so no route shadows a more specific one after it. A parameter next to a
// literal comes before one alone, the longer literal first.
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

// Unmarked writes each colon and star of name, which routers read as a pattern, as an underscore.
func Unmarked(name string) string {
	return strings.NewReplacer(":", "_", "*", "_").Replace(name)
}

// CheckMethod fails on a method a router with one function per method has none for, such as
// QUERY or one a spec adds on its own.
func CheckMethod(method string) error {
	if !slices.Contains(methods, method) {
		return fmt.Errorf("%w %s", ErrMethod, method)
	}
	return nil
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

// Brace writes path for a router that takes {name} too, with names and a trailing /* as given.
func Brace(path string, name, wildcard func(string) string) (string, error) {
	if err := Check(path); err != nil {
		return "", err
	}
	out, given, err := Rename(path, name)
	if err != nil {
		return "", err
	}
	switch rest, isWildcard := strings.CutSuffix(out, "/*"); {
	case isWildcard:
		return rest + "/" + wildcard(RestName(given)), nil
	case strings.HasSuffix(out, "*"):
		return "", fmt.Errorf("%w: * must be a segment of its own", ErrPattern)
	}
	return out, nil
}

// Rename writes each {name} of path as name gives it; a missing, repeated or merged name fails.
func Rename(path string, name func(string) string) (string, []string, error) {
	var names, given []string
	for _, n := range Params(path) {
		if n == "" {
			return "", nil, fmt.Errorf("%w: a parameter has no name", ErrPattern)
		}
		if err := checkName(names, n, given, name(n)); err != nil {
			return "", nil, err
		}
		names, given = append(names, n), append(given, name(n))
	}
	out := param.ReplaceAllStringFunc(path, func(m string) string { return "{" + name(m[1:len(m)-1]) + "}" })
	return out, given, nil
}

// RestName names the value a trailing * takes the rest of the path into: rest, with an
// underscore added for as long as a parameter of the path has the name.
func RestName(taken []string) string {
	name := "rest"
	for slices.Contains(taken, name) {
		name += "_"
	}
	return name
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

// EscapingColon escapes the colons of a literal, and fails on a leading colon or a star.
func EscapingColon(s string) (string, error) {
	switch {
	case strings.HasPrefix(s, ":"):
		return "", fmt.Errorf("%w: a segment beginning with : is read as a parameter", ErrPattern)
	case strings.Contains(s, "*"):
		return "", fmt.Errorf("%w: * is read as a wildcard in %s", ErrPattern, s)
	}
	return Escaping(":")(s)
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

// checkName fails on a name given twice, or on one the router names as an earlier one.
func checkName(names []string, name string, given []string, as string) error {
	switch i := slices.Index(given, as); {
	case slices.Contains(names, name):
		return fmt.Errorf("%w: parameter %q is named twice", ErrPattern, name)
	case i >= 0:
		return fmt.Errorf("%w: parameters %q and %q are both %s on the router", ErrPattern, names[i], name, as)
	}
	return nil
}

// ranks is how general each segment of path is, two numbers each, in the order StaticFirst sorts by.
func ranks(path string) []int {
	var out []int
	for _, seg := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		literal := len(param.ReplaceAllString(seg, ""))
		switch {
		case seg == "*":
			out = append(out, 3, 0)
		case !strings.Contains(seg, "{"):
			out = append(out, 0, 0)
		case literal > 0:
			out = append(out, 1, -literal)
		default:
			out = append(out, 2, 0)
		}
	}
	return out
}

// httpParams are the parameters of an http.HandlerFunc, w and r.
func httpParams(s *gocode.Scope) []string {
	pkg := s.Import(gomodel.Import{Path: "net/http"})
	return []string{gocode.Param("w", gocode.Selector(pkg, "ResponseWriter")), gocode.Param("r", gocode.Deref(gocode.Selector(pkg, "Request")))}
}
