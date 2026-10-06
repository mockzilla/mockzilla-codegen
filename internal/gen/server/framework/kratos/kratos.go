// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package kratos is the router for the HTTP transport of github.com/go-kratos/kratos/v2, whose
// handlers take a kratos Context and return an error.
package kratos

import (
	"embed"
	"fmt"
	"io/fs"
	"path"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/go-kratos/kratos/v2/transport/http"

//go:embed templates/*.tmpl
var templates embed.FS

var _ framework.Framework = Framework{}

// Framework is the kratos router.
type Framework struct{}

func (Framework) Name() string {
	return "kratos"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath, Alias: "khttp"}}
}

// RoutePattern writes the path as for mux, which kratos routes with; the path must be clean.
func (Framework) RoutePattern(method, oasPath string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	if oasPath != path.Clean(oasPath) {
		return "", fmt.Errorf("%w: it is not a clean path", framework.ErrPattern)
	}
	return framework.Brace(oasPath, framework.Unmarked, wildcard)
}

// Conflicts drops every route with the shape of an earlier one and orders the rest as mux tries them.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	kept, dropped := framework.ConflictsByShape(routes)
	return framework.StaticFirst(kept), dropped
}

// Handler is kratos's own shape, a handler of its context.
func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.ContextHandler(gocode.Selector(s.Import(gomodel.Import{Path: importPath, Alias: "khttp"}), "Context"))
}

func (Framework) PathParam(_ *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector(gocode.Call(gocode.Selector("c", "Vars")), "Get"), gocode.Quote(framework.Unmarked(name)))
}

func (Framework) Templates() fs.FS {
	return templates
}

// wildcard is the parameter a trailing /* becomes, which takes the rest of the path.
func wildcard(name string) string {
	return "{" + name + ":.*}"
}
