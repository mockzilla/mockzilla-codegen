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

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}}
}

// RoutePattern keeps the path, a colon in a name as an underscore, since chi reads it as a pattern.
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
	out, _, err := framework.Rename(path, framework.Unmarked)
	return out, err
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, which chi keys by position.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByShape(routes)
}

func (Framework) Handler(s *gocode.Scope) framework.Handler {
	return framework.HTTPHandler(s)
}

// PathParam unescapes the value, which chi cuts from the raw path when the request has one.
func (Framework) PathParam(s *gocode.Scope, name string) string {
	value := gocode.Call(gocode.Selector(s.Import(gomodel.Import{Path: importPath}), "URLParam"), "r", gocode.Quote(framework.Unmarked(name)))
	return gocode.Call(gocode.Selector(s.Import(gomodel.Import{Path: gomodel.RuntimePath}), "UnescapePath"), "r", value)
}

func (Framework) Templates() fs.FS {
	return templates
}
