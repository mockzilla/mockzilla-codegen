// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package echo is the router for github.com/labstack/echo, whose handlers take an echo.Context and
// return an error.
package echo

import (
	"embed"
	"io/fs"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/labstack/echo/v4"

//go:embed templates/*.tmpl
var templates embed.FS

// pattern writes routes as echo takes them.
var pattern = framework.Colon{Literal: framework.Escaping(":"), Name: framework.Same, Wildcard: "*", IsPrefixAllowed: true}

var _ framework.Framework = Framework{}

// Framework is the echo router.
type Framework struct{}

func (Framework) Name() string {
	return "echo"
}

func (Framework) Family() framework.Family {
	return framework.Native
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}}
}

// RoutePattern writes each parameter as :name and escapes a literal colon as \:, since echo reads
// a colon as the start of a parameter. A parameter runs to the end of its segment on echo, so a
// path fails when one has a suffix or shares a segment with another; it also fails on a path
// without a leading slash, an unclosed brace, a parameter without a name or named twice, and a
// wildcard that is not a segment of its own, last.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	return pattern.Pattern(path)
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, since echo keys parameters by position and
// lets a later route replace an earlier one without a word.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByShape(routes)
}

// Handler is echo's own shape: the handler takes the context c and returns an error, which stays
// nil since the error handler writes every failed request.
func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.Handler{
		Signature: "(c " + s.Import(gomodel.Import{Path: importPath}) + ".Context) error",
		Prologue:  "w, r := c.Response(), c.Request()",
		Return:    "return nil",
		Epilogue:  "return nil",
	}
}

func (Framework) PathParam(_ *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector("c", "Param"), gocode.Quote(name))
}

func (Framework) Templates() fs.FS {
	return templates
}
