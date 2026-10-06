// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package echov5 is the router for github.com/labstack/echo/v5, whose handlers take an
// *echo.Context and return an error.
package echov5

import (
	"embed"
	"io/fs"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/labstack/echo/v5"

//go:embed templates/*.tmpl
var templates embed.FS

// pattern writes routes as echo takes them.
var pattern = framework.Colon{Literal: framework.EscapingColon, Name: framework.Same, Wildcard: "*", IsPrefixAllowed: true}

var _ framework.Framework = Framework{}

// Framework is the echo v5 router.
type Framework struct{}

func (Framework) Name() string {
	return "echo-v5"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}}
}

// RoutePattern writes each parameter as :name and a literal colon as \:, which echo would misread.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return pattern.Pattern(path)
}

// Conflicts drops every route with the method and shape of an earlier one, which echo would replace.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByShape(routes)
}

// Handler is echo's own shape, a handler of its context.
func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.ContextHandler(gocode.Deref(gocode.Selector(s.Import(gomodel.Import{Path: importPath}), "Context")))
}

// PathParam unescapes the value, which echo cuts from the raw path when the request has one.
func (Framework) PathParam(s *gocode.Scope, name string) string {
	value := gocode.Call(gocode.Selector("c", "Param"), gocode.Quote(name))
	return gocode.Call(gocode.Selector(s.Import(gomodel.Import{Path: gomodel.RuntimePath}), "UnescapePath"), "r", value)
}

func (Framework) Templates() fs.FS {
	return templates
}
