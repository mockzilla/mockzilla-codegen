// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package echo is the router for github.com/labstack/echo, whose handlers take an echo.Context and
// return an error.
package echo

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/labstack/echo/v4"

//go:embed *.tmpl
var templates embed.FS

// paramSegment is a segment that ends in one parameter, with a literal prefix or without.
var paramSegment = regexp.MustCompile(`^[^{}]*\{([^{}]*)\}$`)

var _ framework.Framework = Framework{}

// Framework is the echo router.
type Framework struct{}

func (Framework) Name() string {
	return "echo"
}

func (Framework) Family() framework.Family {
	return framework.Native
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}}
}

// RoutePattern writes each parameter as :name and escapes a literal colon as \:, since echo reads
// a colon as the start of a parameter. A parameter runs to the end of its segment on echo, so a
// path fails when one has a suffix or shares a segment with another; it also fails on a path
// without a leading slash, an unclosed brace, a parameter without a name or named twice, and a
// wildcard that is not last.
func (Framework) RoutePattern(_, path string) (string, error) {
	switch {
	case !strings.HasPrefix(path, "/"):
		return "", fmt.Errorf("%w: it must begin with /", framework.ErrPattern)
	case strings.Count(path, "{") != strings.Count(path, "}"):
		return "", fmt.Errorf("%w: a { has no }", framework.ErrPattern)
	}
	if i := strings.Index(path, "*"); i >= 0 && i != len(path)-1 {
		return "", fmt.Errorf("%w: * must be last", framework.ErrPattern)
	}

	segments := strings.Split(path[1:], "/")
	var names []string
	for i, seg := range segments {
		if !strings.ContainsAny(seg, "{}") {
			segments[i] = literal(seg)
			continue
		}
		m := paramSegment.FindStringSubmatch(seg)
		switch {
		case m == nil:
			return "", fmt.Errorf("%w: a parameter must end its segment, unlike %s", framework.ErrPattern, seg)
		case m[1] == "":
			return "", fmt.Errorf("%w: a parameter has no name", framework.ErrPattern)
		case slices.Contains(names, m[1]):
			return "", fmt.Errorf("%w: parameter %q is named twice", framework.ErrPattern, m[1])
		}
		names = append(names, m[1])
		segments[i] = literal(strings.TrimSuffix(seg, "{"+m[1]+"}")) + ":" + m[1]
	}
	return "/" + strings.Join(segments, "/"), nil
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, since echo keys parameters by position and
// lets a later route replace an earlier one without a word.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByShape(routes)
}

// Handler is echo's own shape: the handler takes the context c and returns an error, which stays
// nil since the error handler writes every failed request.
func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.Handler{
		Signature: "(c " + s.Import(gomodel.Import{Path: importPath}) + ".Context) error",
		Prologue:  "w, r := c.Response(), c.Request()",
		Return:    "return nil",
		Epilogue:  "return nil",
	}
}

func (Framework) PathParam(_ *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector("c", "Param"), gocode.Quote(name))
}

func (Framework) Templates() fs.FS {
	return templates
}

// literal writes a piece of the path echo takes as it is, with every colon escaped.
func literal(s string) string {
	return strings.ReplaceAll(s, ":", `\:`)
}
