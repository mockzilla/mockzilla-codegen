// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package gozero is the router for the rest package of github.com/zeromicro/go-zero: the router
// of its rest server, which rest.WithRouter gives one.
package gozero

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const (
	importPath = "github.com/zeromicro/go-zero/rest/httpx"
	varsPath   = "github.com/zeromicro/go-zero/rest/pathvar"
)

//go:embed templates/*.tmpl
var templates embed.FS

// pattern writes routes as go-zero takes them: a parameter fills its segment, and a literal
// segment must not begin with a colon, which marks a parameter.
var pattern = framework.Colon{Literal: literal, Name: framework.Same}

var _ framework.Framework = Framework{}

// Framework is the go-zero router.
type Framework struct{}

func (Framework) Name() string {
	return "go-zero"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{
		{Path: importPath},
		{Path: "github.com/zeromicro/go-zero/rest/router"},
		{Path: varsPath},
		{Path: "net/http"},
	}
}

// RoutePattern writes each parameter as :name without a trailing slash, and fails on TRACE and a *.
func (Framework) RoutePattern(method, oasPath string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}
	if method == http.MethodTrace {
		return "", fmt.Errorf("%w %s", framework.ErrMethod, method)
	}

	if strings.Contains(oasPath, "*") {
		return "", fmt.Errorf("%w: the router has no wildcard", framework.ErrPattern)
	}
	trimmed := oasPath
	if trimmed != "/" {
		trimmed = strings.TrimSuffix(trimmed, "/")
	}
	if trimmed != path.Clean(trimmed) {
		return "", fmt.Errorf("%w: it is not a clean path", framework.ErrPattern)
	}
	return pattern.Pattern(trimmed)
}

// Conflicts drops every route go-zero refuses as a duplicate of an earlier one, trailing slash aside.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByKey(routes, func(r framework.Route) string {
		return r.Method + " " + strings.TrimSuffix(framework.Shape(r.Path), "/")
	})
}

func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.HTTPHandler(s)
}

func (Framework) PathParam(s *gocode.Scope, name string) string {
	vars := gocode.Call(gocode.Selector(s.Import(gomodel.Import{Path: varsPath}), "Vars"), "r")
	return gocode.Index(vars, gocode.Quote(name))
}

func (Framework) Templates() fs.FS {
	return templates
}

// literal writes a literal segment as it is, or fails for one that begins with a colon.
func literal(s string) (string, error) {
	if strings.HasPrefix(s, ":") {
		return "", fmt.Errorf("%w: a segment beginning with : is read as a parameter", framework.ErrPattern)
	}
	return s, nil
}
