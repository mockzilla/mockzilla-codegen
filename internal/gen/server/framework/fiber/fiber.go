// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package fiber is the router for github.com/gofiber/fiber/v3. The handlers stay
// http.HandlerFuncs, served through fasthttp's adaptor with the path parameters on the request,
// and the app is served in fiber's own way.
package fiber

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/gofiber/fiber/v3"

//go:embed *.tmpl
var templates embed.FS

// pattern writes routes as fiber takes them: the characters fiber reads as the start of a
// parameter are escaped, and a parameter name ends at a character that is no identifier.
var pattern = framework.Colon{Literal: framework.Escaping(":*+?"), Name: framework.Identifier, Wildcard: "*", IsPrefixAllowed: true}

var _ framework.Framework = Framework{}

// Framework is the fiber router.
type Framework struct{}

func (Framework) Name() string {
	return "fiber"
}

func (Framework) Family() framework.Family {
	return framework.NetHTTP
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{
		{Path: importPath},
		{Path: "github.com/valyala/fasthttp/fasthttpadaptor"},
		{Path: "net/http"},
		{Path: "strings"},
	}
}

// RoutePattern writes each parameter as :name, with a name that is an identifier since fiber
// ends a name at any other character, and escapes a literal colon, star, plus or question mark
// with a backslash, since fiber reads them as the start of a parameter. A parameter runs to the
// end of its segment, so a path fails when one has a suffix or shares a segment with another; a
// path also fails without a leading slash, with an unclosed brace, a parameter without a name or
// named twice, and a wildcard that is not a segment of its own, last.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return pattern.Pattern(path)
}

// Conflicts drops every route that matches the same requests as an earlier one of its method: a
// repeat of it, one whose path parameters are named otherwise, or one that differs in the
// trailing slash alone, which fiber does not tell apart. It then puts literals before parameters
// and parameters before the wildcard at each position, since fiber takes the first route that
// matches.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	kept, dropped := framework.ConflictsByKey(routes, func(r framework.Route) string {
		return r.Method + " " + strings.TrimSuffix(framework.Shape(r.Path), "/")
	})
	return framework.StaticFirst(kept), dropped
}

func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.HTTPHandler(s)
}

func (Framework) PathParam(_ *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector("r", "PathValue"), gocode.Quote(framework.Identifier(name)))
}

func (Framework) Templates() fs.FS {
	return templates
}
