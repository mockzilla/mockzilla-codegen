// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package beego is the router for github.com/beego/beego/v2. The handlers stay http.HandlerFuncs,
// served from beego's context with the path parameters on the request.
package beego

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/beego/beego/v2/server/web"

//go:embed templates/*.tmpl
var templates embed.FS

// pattern writes routes as beego takes them: a literal colon, star or question mark cannot be
// escaped, a parameter fills its segment and its name is an identifier.
var pattern = framework.Colon{Literal: framework.Rejecting(":*?"), Name: framework.Identifier, Wildcard: "*"}

var _ framework.Framework = Framework{}

// Framework is the beego router.
type Framework struct{}

func (Framework) Name() string {
	return "beego"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{
		{Path: importPath},
		{Path: importPath + "/context", Alias: "bcontext"},
		{Path: "io"},
		{Path: "net/http"},
		{Path: "strings"},
	}
}

// RoutePattern writes each parameter as :name, with the name an identifier, as beego takes no other.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return pattern.Pattern(path)
}

// Conflicts drops every route with the method and shape of an earlier one, trailing slash aside.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByKey(routes, func(r framework.Route) string {
		return r.Method + " " + strings.TrimSuffix(framework.Shape(r.Path), "/")
	})
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
