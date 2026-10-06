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

//go:embed templates/*.tmpl
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

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{
		{Path: importPath},
		{Path: "errors"},
		{Path: "github.com/valyala/fasthttp/fasthttpadaptor"},
		{Path: "net/http"},
		{Path: "strings"},
	}
}

// RoutePattern writes each parameter as :name, an identifier, and escapes what fiber reads as one.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return pattern.Pattern(path)
}

// Conflicts drops every route fiber cannot tell from an earlier one and orders the rest for it.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	kept, dropped := framework.ConflictsByKey(routes, func(r framework.Route) string {
		return r.Method + " " + strings.ToLower(strings.TrimSuffix(framework.Shape(r.Path), "/"))
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
