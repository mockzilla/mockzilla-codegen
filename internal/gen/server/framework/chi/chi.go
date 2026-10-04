// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package chi is the router for github.com/go-chi/chi.
package chi

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

const importPath = "github.com/go-chi/chi/v5"

//go:embed templates/*.tmpl
var templates embed.FS

var _ framework.Framework = Framework{}

// Framework is the chi router.
type Framework struct{}

func (Framework) Name() string {
	return "chi"
}

func (Framework) Family() framework.Family {
	return framework.NetHTTP
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}}
}

// RoutePattern keeps the path as it is, since chi writes parameters as {name} too and takes the
// method as a call of its own. It fails on what chi panics on: a path without a leading slash, an
// unclosed brace, a parameter named twice, or a wildcard that is not last.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	switch {
	case !strings.HasPrefix(path, "/"):
		return "", fmt.Errorf("%w: it must begin with /", framework.ErrPattern)
	case strings.Count(path, "{") != strings.Count(path, "}"):
		return "", fmt.Errorf("%w: a { has no }", framework.ErrPattern)
	}
	if i := strings.Index(path, "*"); i >= 0 && i != len(path)-1 {
		return "", fmt.Errorf("%w: * must be last", framework.ErrPattern)
	}

	names := framework.Params(path)
	for i, name := range names {
		if slices.Contains(names[:i], name) {
			return "", fmt.Errorf("%w: parameter %q is named twice", framework.ErrPattern, name)
		}
	}
	return path, nil
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, which chi keys by position.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByShape(routes)
}

func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.HTTPHandler(s)
}

func (Framework) PathParam(s *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector(s.Import(gomodel.Import{Path: importPath}), "URLParam"), "r", gocode.Quote(name))
}

func (Framework) Templates() fs.FS {
	return templates
}
