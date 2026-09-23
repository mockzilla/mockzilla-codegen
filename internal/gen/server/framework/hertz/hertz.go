// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package hertz is the router for github.com/cloudwego/hertz. The handlers stay
// http.HandlerFuncs, served from hertz's request context with the path parameters on the
// request, and the server is served in hertz's own way.
package hertz

import (
	"embed"
	"io/fs"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/cloudwego/hertz/pkg/app/server"

//go:embed *.tmpl
var templates embed.FS

// pattern writes routes as hertz takes them: a literal colon or star cannot be escaped, and the
// wildcard needs a name.
var pattern = framework.Colon{Literal: framework.Rejecting(":*"), Name: framework.Same, Wildcard: "*rest", IsPrefixAllowed: true}

var _ framework.Framework = Framework{}

// Framework is the hertz router.
type Framework struct{}

func (Framework) Name() string {
	return "hertz"
}

func (Framework) Family() framework.Family {
	return framework.NetHTTP
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{
		{Path: importPath},
		{Path: "github.com/cloudwego/hertz/pkg/app"},
		{Path: "github.com/cloudwego/hertz/pkg/protocol"},
		{Path: "bytes"},
		{Path: "context"},
		{Path: "net/http"},
	}
}

// RoutePattern writes each parameter as :name and a trailing /* as /*rest, the catch-all hertz
// asks a name for, as for gin: a parameter runs to the end of its segment, so a path fails when
// one has a suffix or shares a segment with another, and a literal colon or star fails since
// hertz reads them as the start of a parameter; a path also fails without a leading slash, with
// an unclosed brace, a parameter without a name or named twice, and a wildcard that is not a
// segment of its own, last.
func (Framework) RoutePattern(_, path string) (string, error) {
	return pattern.Pattern(path)
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, which hertz panics on as a registered path.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByShape(routes)
}

func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.HTTPHandler(s)
}

func (Framework) PathParam(_ *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector("r", "PathValue"), gocode.Quote(name))
}

func (Framework) Templates() fs.FS {
	return templates
}
