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
	"unicode"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

const (
	importPath = "net/http"
	// restName names the wildcard a trailing * becomes.
	restName = "rest"
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

// RoutePattern writes METHOD /path with each parameter as a wildcard, a trailing * as the
// wildcard {rest...} and a trailing slash as {$}, so the route matches the path exactly. It fails
// on what ServeMux panics on: a path without a leading slash, one that is not clean, a parameter
// that does not fill its segment or has no name, and two wildcards of one name.
func (Framework) RoutePattern(method, oasPath string) (string, error) {
	switch {
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
			segments[i] = "{" + restName + "...}"
			names = append(names, restName)
		case !strings.ContainsAny(seg, "{}"):
		case !wildcardSegment.MatchString(seg):
			return "", fmt.Errorf("%w: a parameter must fill its segment, unlike %s", framework.ErrPattern, seg)
		default:
			name := wildcard(seg[1 : len(seg)-1])
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

func (Framework) PathParam(_ *gocode.Scope, name string) string {
	return gocode.Call(gocode.Selector("r", "PathValue"), gocode.Quote(wildcard(name)))
}

func (Framework) Templates() fs.FS {
	return templates
}

// wildcard is the name a path parameter has on the mux. ServeMux takes Go identifiers, so any
// other character becomes an underscore, and a leading digit gets one in front.
func wildcard(name string) string {
	var b strings.Builder
	for i, c := range name {
		switch {
		case unicode.IsLetter(c) || c == '_' || (i > 0 && unicode.IsDigit(c)):
			b.WriteRune(c)
		case unicode.IsDigit(c):
			b.WriteRune('_')
			b.WriteRune(c)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// cleanPath is the path as ServeMux cleans it, which keeps a trailing slash.
func cleanPath(p string) string {
	np := path.Clean(p)
	if strings.HasSuffix(p, "/") && np != "/" {
		np += "/"
	}
	return np
}
