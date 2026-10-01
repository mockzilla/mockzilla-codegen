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

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/beego/beego/v2/server/web"

//go:embed *.tmpl
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

func (Framework) Family() framework.Family {
	return framework.NetHTTP
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

// RoutePattern writes each parameter as :name, with a name that is an identifier since beego
// ends a name at any other character, and a trailing /* as beego's own. A parameter fills its
// segment on beego, so a path fails when one has a prefix or a suffix or shares a segment with
// another, and a literal colon, star or question mark fails since beego reads them as the start
// of a parameter; a path also fails without a leading slash, with an unclosed brace, a parameter
// without a name or named twice, and a wildcard that is not a segment of its own, last.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return pattern.Pattern(path)
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, since beego holds one route of a shape and
// takes literals before parameters on its own.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByShape(routes)
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
