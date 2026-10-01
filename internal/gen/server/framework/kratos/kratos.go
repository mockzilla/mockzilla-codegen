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

//go:embed *.tmpl
var templates embed.FS

var _ framework.Framework = Framework{}

// Framework is the kratos router.
type Framework struct{}

func (Framework) Name() string {
	return "kratos"
}

func (Framework) Family() framework.Family {
	return framework.Native
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath, Alias: "khttp"}}
}

// RoutePattern keeps the path as it is, since kratos routes with gorilla/mux and writes
// parameters as {name} too, with a trailing /* as /{rest:.*}. It fails on a path that is not
// clean, such as one with a trailing slash, which kratos cleans without a word, and on what mux
// rejects or misreads: a path without a leading slash, an unclosed brace, a wildcard that is not
// a segment of its own, last, a parameter without a name, one whose name holds a colon, and a
// parameter named twice.
func (Framework) RoutePattern(method, oasPath string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	if oasPath != path.Clean(oasPath) {
		return "", fmt.Errorf("%w: it is not a clean path", framework.ErrPattern)
	}
	return framework.Brace(oasPath, "{rest:.*}")
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, since mux takes the first route that
// matches. For the same reason it puts literals before parameters and parameters before the
// wildcard at each position.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	kept, dropped := framework.ConflictsByShape(routes)
	return framework.StaticFirst(kept), dropped
}

// Handler is kratos's own shape: the handler takes the context c and returns an error, which
// stays nil since the error handler writes every failed request.
func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.Handler{
		Signature: "(c " + s.Import(gomodel.Import{Path: importPath, Alias: "khttp"}) + ".Context) error",
		Prologue:  "w, r := c.Response(), c.Request()",
		Return:    "return nil",
		Epilogue:  "return nil",
	}
}

func (Framework) PathParam(_ *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector(gocode.Call(gocode.Selector("c", "Vars")), "Get"), gocode.Quote(name))
}

func (Framework) Templates() fs.FS {
	return templates
}
