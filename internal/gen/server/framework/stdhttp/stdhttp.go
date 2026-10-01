// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package stdhttp is the router for http.ServeMux of the standard library, with the method and
// wildcard patterns of Go 1.22.
package stdhttp

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const (
	importPath = "net/http"
	// methodMarks are the characters a method may hold besides letters and digits.
	methodMarks = "!#$%&'*+-.^_`|~"
)

//go:embed *.tmpl
var templates embed.FS

// wildcardSegment is a segment that is one parameter and nothing else.
var wildcardSegment = regexp.MustCompile(`^\{[^{}]*\}$`)

var _ framework.Framework = Framework{}

// Framework is the http.ServeMux router.
type Framework struct{}

func (Framework) Name() string {
	return "std-http"
}

func (Framework) Family() framework.Family {
	return framework.NetHTTP
}

func (Framework) Imports() []gomodel.Import {
	return []gomodel.Import{{Path: importPath}}
}

// RoutePattern writes METHOD /path so the route matches the path and nothing else: a parameter as
// a wildcard named as an identifier, a trailing * as {rest...}, a trailing slash as {$}. It fails
// on what ServeMux panics on.
func (Framework) RoutePattern(method, oasPath string) (string, error) {
	switch {
	case !isToken(method):
		return "", fmt.Errorf("%w %q", framework.ErrMethod, method)
	case !strings.HasPrefix(oasPath, "/"):
		return "", fmt.Errorf("%w: it must begin with /", framework.ErrPattern)
	case oasPath != cleanPath(oasPath):
		return "", fmt.Errorf("%w: it is not a clean path", framework.ErrPattern)
	}

	segments := strings.Split(oasPath[1:], "/")
	var names []string
	for i, seg := range segments {
		isLast := i == len(segments)-1
		switch {
		case seg == "" && isLast:
			segments[i] = "{$}"
		case seg == "*" && isLast:
			segments[i] = "{" + restName(names) + "...}"
		case !strings.Contains(seg, "{"):
		case len(framework.Params(seg)) == 0:
			return "", fmt.Errorf("%w: a { has no }", framework.ErrPattern)
		case !wildcardSegment.MatchString(seg):
			return "", fmt.Errorf("%w: a parameter must fill its segment, unlike %s", framework.ErrPattern, seg)
		default:
			name := framework.Identifier(seg[1 : len(seg)-1])
			if name == "" {
				return "", fmt.Errorf("%w: a parameter has no name", framework.ErrPattern)
			}
			segments[i] = "{" + name + "}"
			names = append(names, name)
		}
	}

	for i, name := range names {
		if slices.Contains(names[:i], name) {
			return "", fmt.Errorf("%w: the wildcard %q is named twice", framework.ErrPattern, name)
		}
	}
	return method + " /" + strings.Join(segments, "/"), nil
}

// Conflicts drops every route ServeMux panics on next to an earlier one: a route that matches the
// same requests as it, or one that overlaps with it while neither is more specific.
func (Framework) Conflicts(routes []framework.Route) ([]framework.Route, []framework.Conflict) {
	var kept []framework.Route
	var dropped []framework.Conflict
	x := newIndex()
	for _, r := range routes {
		p := parse(r.Pattern)
		if reason := x.conflict(p, kept); reason != "" {
			dropped = append(dropped, framework.Conflict{Route: r, Reason: reason})
			continue
		}
		x.add(p)
		kept = append(kept, r)
	}
	return kept, dropped
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

// isToken reports whether method is an HTTP token, which is all ServeMux asks of a method.
func isToken(method string) bool {
	isRejected := func(r rune) bool {
		isAlnum := 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9'
		return !isAlnum && !strings.ContainsRune(methodMarks, r)
	}
	return method != "" && !strings.ContainsFunc(method, isRejected)
}

// cleanPath is the path as ServeMux cleans it, which keeps a trailing slash.
func cleanPath(p string) string {
	np := path.Clean(p)
	if strings.HasSuffix(p, "/") && np != "/" {
		np += "/"
	}
	return np
}

// restName names the wildcard a trailing * becomes: rest, with an underscore added for as long as
// a parameter of the path has the name.
func restName(taken []string) string {
	name := "rest"
	for slices.Contains(taken, name) {
		name += "_"
	}
	return name
}
