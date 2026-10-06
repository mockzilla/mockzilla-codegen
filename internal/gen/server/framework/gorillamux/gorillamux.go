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

//go:embed templates/*.tmpl
var templates embed.FS

var _ framework.Framework = Framework{}

// Framework is the gorilla/mux router.
type Framework struct{}

func (Framework) Name() string {
	return "gorilla-mux"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}, {Path: "net/http"}}
}

// RoutePattern keeps {name} parameters, a colon in a name as an underscore, and a /* as /{rest:.*}.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return framework.Brace(path, framework.Unmarked, wildcard)
}

// Conflicts drops every route with the shape of an earlier one and orders the rest as mux tries them.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	kept, dropped := framework.ConflictsByShape(routes)
	return framework.StaticFirst(kept), dropped
}

func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.HTTPHandler(s)
}

func (Framework) PathParam(s *gocode.Scope, name string) string {
	vars := gocode.Call(gocode.Selector(s.Import(gomodel.Import{Path: importPath}), "Vars"), "r")
	return gocode.Index(vars, gocode.Quote(framework.Unmarked(name)))
}

func (Framework) Templates() fs.FS {
	return templates
}

// wildcard is the parameter a trailing /* becomes, which takes the rest of the path.
func wildcard(name string) string {
	return "{" + name + ":.*}"
}
