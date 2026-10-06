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
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const (
	importPath = "github.com/gogf/gf/v2/net/ghttp"
	// regexpMarks are the characters of a regular expression GoFrame does not quote in a literal.
	regexpMarks = `$^()|[]?\`
)

//go:embed templates/*.tmpl
var templates embed.FS

var _ framework.Framework = Framework{}

// Framework is the GoFrame router.
type Framework struct{}

func (Framework) Name() string {
	return "goframe"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{
		{Path: importPath},
		{Path: "github.com/gogf/gf/v2/text/gregex"},
		{Path: "github.com/gogf/gf/v2/util/guid"},
		{Path: "net/http"},
		{Path: "net/url"},
	}
}

// RoutePattern writes METHOD:/path with {name} fields as identifiers and a trailing /* as /*rest.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	if err := framework.Check(path); err != nil {
		return "", err
	}
	out, given, err := framework.Rename(path, framework.Identifier)
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(path, "*") && !strings.HasSuffix(path, "/*") {
		return "", fmt.Errorf("%w: * must be a segment of its own", framework.ErrPattern)
	}
	literals := framework.Shape(path)
	if strings.ContainsAny(literals, ":@") {
		return "", fmt.Errorf("%w: a colon or an at sign is read as the start of a parameter or a domain", framework.ErrPattern)
	}
	if i := strings.IndexAny(literals, regexpMarks); i >= 0 {
		return "", fmt.Errorf("%w: %s is read as part of a regular expression", framework.ErrPattern, literals[i:i+1])
	}

	if rest, isWildcard := strings.CutSuffix(out, "/*"); isWildcard {
		out = rest + "/*" + framework.RestName(given)
	}
	if out != "/" {
		out = strings.TrimSuffix(out, "/")
	}
	return method + ":" + out, nil
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
