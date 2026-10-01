// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package goframe is the router for github.com/gogf/gf/v2. The handlers stay http.HandlerFuncs,
// served from GoFrame's request with the path parameters on it, and the server is served in
// GoFrame's own way.
package goframe

import (
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/gogf/gf/v2/net/ghttp"

//go:embed *.tmpl
var templates embed.FS

var _ framework.Framework = Framework{}

// Framework is the GoFrame router.
type Framework struct{}

func (Framework) Name() string {
	return "goframe"
}

func (Framework) Family() framework.Family {
	return framework.NetHTTP
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}, {Path: "github.com/gogf/gf/v2/util/guid"}, {Path: "net/http"}}
}

// RoutePattern writes METHOD:/path with each parameter as a {name} field whose name is an
// identifier, a trailing /* as the fuzzy /*rest and no trailing slash, which GoFrame drops. It
// fails on a path without a leading slash, an unclosed brace, a wildcard that is not a segment
// of its own, last, a parameter without a name or named twice, and a literal colon, star or at
// sign, which GoFrame reads as the start of a parameter or of a domain.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	if err := framework.Check(path); err != nil {
		return "", err
	}
	names := framework.Params(path)
	for i, name := range names {
		switch {
		case name == "":
			return "", fmt.Errorf("%w: a parameter has no name", framework.ErrPattern)
		case slices.Contains(names[:i], name):
			return "", fmt.Errorf("%w: parameter %q is named twice", framework.ErrPattern, name)
		}
	}
	if strings.HasSuffix(path, "*") && !strings.HasSuffix(path, "/*") {
		return "", fmt.Errorf("%w: * must be a segment of its own", framework.ErrPattern)
	}
	if literals := framework.Shape(path); strings.ContainsAny(literals, ":@") {
		return "", fmt.Errorf("%w: a colon or an at sign is read as the start of a parameter or a domain", framework.ErrPattern)
	}

	for _, name := range names {
		path = strings.Replace(path, "{"+name+"}", "{"+framework.Identifier(name)+"}", 1)
	}
	if rest, isWildcard := strings.CutSuffix(path, "/*"); isWildcard {
		path = rest + "/*rest"
	}
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	return method + ":" + path, nil
}

// Conflicts drops every route that matches the same requests as an earlier one of its method: a
// repeat of it, one whose path parameters are named otherwise, or one that differs in the
// trailing slash alone, since GoFrame exits on a route registered twice and drops the slash.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByKey(routes, func(r framework.Route) string { return framework.Shape(r.Pattern) })
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
