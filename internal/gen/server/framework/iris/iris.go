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
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const importPath = "github.com/kataras/iris/v12"

//go:embed templates/*.tmpl
var templates embed.FS

// wholeSegment is a segment that is one parameter and nothing else.
var wholeSegment = regexp.MustCompile(`^\{[^{}]*\}$`)

var _ framework.Framework = Framework{}

// Framework is the iris router.
type Framework struct{}

func (Framework) Name() string {
	return "iris"
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}, {Path: "context"}, {Path: "net/http"}}
}

// RoutePattern writes each parameter as an identifier {name}, a /* as {rest:path}, no trailing slash.
func (Framework) RoutePattern(method, path string) (string, error) {
	if err := framework.CheckMethod(method); err != nil {
		return "", err
	}

	if err := framework.Check(path); err != nil {
		return "", err
	}
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	out, given, err := framework.Rename(path, framework.Identifier)
	if err != nil {
		return "", err
	}

	segments, renamed := strings.Split(path[1:], "/"), strings.Split(out[1:], "/")
	for i, seg := range segments {
		switch {
		case seg == "*" && i == len(segments)-1:
			renamed[i] = "{" + framework.RestName(given) + ":path}"
		case strings.HasPrefix(seg, ":"):
			return "", fmt.Errorf("%w: a segment beginning with : is read as a parameter", framework.ErrPattern)
		case !strings.ContainsAny(seg, "{}*"):
		case !wholeSegment.MatchString(seg):
			return "", fmt.Errorf("%w: a parameter must fill its segment, unlike %s", framework.ErrPattern, seg)
		}
	}
	return "/" + strings.Join(renamed, "/"), nil
}

// Conflicts drops every route with the method and shape of an earlier one, trailing slash aside.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	return framework.ConflictsByKey(routes, func(r framework.Route) string {
		return r.Method + " " + strings.TrimSuffix(framework.Shape(r.Path), "/")
	})
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
