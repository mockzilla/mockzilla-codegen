// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package gorillamux is the router for github.com/gorilla/mux.
package gorillamux

import (
	"embed"
	"io/fs"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/gorilla/mux"

//go:embed *.tmpl
var templates embed.FS

var _ framework.Framework = Framework{}

// Framework is the gorilla/mux router.
type Framework struct{}

func (Framework) Name() string {
	return "gorilla-mux"
}

func (Framework) Family() framework.Family {
	return framework.NetHTTP
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}, {Path: "net/http"}}
}

// RoutePattern keeps the path as it is, since mux writes parameters as {name} too, with a
// trailing /* as /{rest:.*}. It fails on what mux rejects or misreads: a path without a leading
// slash, an unclosed brace, a wildcard that is not a segment of its own, last, a parameter
// without a name, one whose name holds a colon, which starts a regular expression, and a
// parameter named twice.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return framework.Brace(path, "{rest:.*}")
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, since mux takes the first route that
// matches. For the same reason it puts literals before parameters and parameters before the
// wildcard at each position.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	kept, dropped := framework.ConflictsByShape(routes)
	return framework.StaticFirst(kept), dropped
}

func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.HTTPHandler(s)
}

func (Framework) PathParam(s *gocode.Scope, name string) string {
	vars := gocode.Call(gocode.Selector(s.Import(gomodel.Import{Path: importPath}), "Vars"), "r")
	return gocode.Index(vars, gocode.Quote(name))
}

func (Framework) Templates() fs.FS {
	return templates
}
