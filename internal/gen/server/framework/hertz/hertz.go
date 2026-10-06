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

//go:embed templates/*.tmpl
var templates embed.FS

// pattern writes routes as hertz takes them: no literal colon or star, and a named wildcard.
var pattern = framework.Colon{Literal: framework.Rejecting(":*"), Name: framework.Unmarked, Wildcard: "*", IsWildcardNamed: true, IsPrefixAllowed: true}

var _ framework.Framework = Framework{}

// Framework is the hertz router.
type Framework struct{}

func (Framework) Name() string {
	return "hertz"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{
		{Path: importPath},
		{Path: "github.com/cloudwego/hertz/pkg/app"},
		{Path: "github.com/cloudwego/hertz/pkg/common/config"},
		{Path: "github.com/cloudwego/hertz/pkg/protocol"},
		{Path: "bytes"},
		{Path: "context"},
		{Path: "net/http"},
	}
}

// RoutePattern writes each parameter as :name and a trailing /* as /*rest, as for gin.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return pattern.Pattern(path)
}

// Conflicts drops every route with the method and shape of an earlier one, which hertz panics on.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByShape(routes)
}

func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.HTTPHandler(s)
}

func (Framework) PathParam(_ *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector("r", "PathValue"), gocode.Quote(framework.Unmarked(name)))
}

func (Framework) Templates() fs.FS {
	return templates
}
