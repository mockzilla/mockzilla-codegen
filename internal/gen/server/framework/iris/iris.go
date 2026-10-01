// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package iris is the router for github.com/kataras/iris/v12. The handlers stay
// http.HandlerFuncs, served from iris's context with the path parameters on the request.
package iris

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/kataras/iris/v12"

//go:embed *.tmpl
var templates embed.FS

// wholeSegment is a segment that is one parameter and nothing else.
var wholeSegment = regexp.MustCompile(`^\{[^{}]*\}$`)

var _ framework.Framework = Framework{}

// Framework is the iris router.
type Framework struct{}

func (Framework) Name() string {
	return "iris"
}

func (Framework) Family() framework.Family {
	return framework.NetHTTP
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}, {Path: "net/http"}}
}

// RoutePattern keeps each parameter as {name}, with a name that is an identifier since iris
// takes no other, and writes a trailing /* as /{rest:path}. A parameter fills its segment on
// iris, so a path fails when one has a prefix or a suffix or shares a segment with another; a
// path also fails without a leading slash, with an unclosed brace, a wildcard that is not a
// segment of its own, last, and a parameter without a name or named twice.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	if err := framework.Check(path); err != nil {
		return "", err
	}

	segments := strings.Split(path[1:], "/")
	var names []string
	for i, seg := range segments {
		isLast := i == len(segments)-1
		switch {
		case seg == "*" && isLast:
			segments[i] = "{rest:path}"
		case !strings.ContainsAny(seg, "{}*"):
		case !wholeSegment.MatchString(seg):
			return "", fmt.Errorf("%w: a parameter must fill its segment, unlike %s", framework.ErrPattern, seg)
		default:
			name := seg[1 : len(seg)-1]
			switch {
			case name == "":
				return "", fmt.Errorf("%w: a parameter has no name", framework.ErrPattern)
			case slices.Contains(names, name):
				return "", fmt.Errorf("%w: parameter %q is named twice", framework.ErrPattern, name)
			}
			names = append(names, name)
			segments[i] = "{" + framework.Identifier(name) + "}"
		}
	}
	return "/" + strings.Join(segments, "/"), nil
}

// Conflicts drops every route that has the method and shape of an earlier one: a repeat of it,
// or one whose path parameters are named otherwise, since iris holds one route of a shape and
// takes literals before parameters on its own.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByShape(routes)
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
