// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of a framework's router.tmpl: one route per operation.

package server

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
)

// shortcuts are the methods routers have a function of their own for.
var shortcuts = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}

// RouterView is the data of the router part. Framework is the name the framework's package is
// imported under and Packages the names of every package the framework imports, by the package's
// own name; the other names are written as the file spells them. User is the config's
// user-context. Runtime and HTTPServer are set when the template writes them.
type RouterView struct {
	Framework  string
	Packages   map[string]string
	Runtime    string
	HTTPServer string
	Service    string
	Option     string
	Options    string
	NewOptions string
	NewAdapter string
	User       map[string]any
	Routes     []RouteView
}

// RouteView registers one operation: Method as the router's method, Get, HTTPMethod as the
// request writes it, GET, and Pattern quoted. IsShortcut marks a method routers have a function
// for, and Names are the route's Names, quoted.
type RouteView struct {
	Method     string
	HTTPMethod string
	Pattern    string
	Operation  string
	IsShortcut bool
	Names      []string
}

func routerView(g *Generator, s *gocode.Scope) *RouterView {
	fw := g.opts.Framework
	v := &RouterView{
		Framework:  s.Import(fw.Imports()[0]),
		Packages:   packages(fw, s, "router.tmpl", g.opts.RouterExtra),
		Service:    s.Symbol(PartService, g.Interface()),
		Option:     s.Symbol(PartAdapter, "ServerOption"),
		Options:    s.Symbol(PartAdapter, "ServerOptions"),
		NewOptions: s.Symbol(PartAdapter, "NewServerOptions"),
		NewAdapter: s.Symbol(PartAdapter, "NewHTTPAdapter"),
		User:       g.opts.User,
	}
	if uses(fw, "router.tmpl", g.opts.RouterExtra, ".Runtime") {
		v.Runtime = s.Import(gomodel.Import{Path: gomodel.RuntimePath})
	}
	if uses(fw, "router.tmpl", g.opts.RouterExtra, ".HTTPServer") {
		v.HTTPServer = s.Import(gomodel.Import{Path: gomodel.HTTPServerPath})
	}
	for _, r := range g.routes {
		route := RouteView{
			Method:     routerMethod(r.Method),
			HTTPMethod: r.Method,
			Pattern:    gocode.Quote(r.Pattern),
			Operation:  r.Operation,
			IsShortcut: slices.Contains(shortcuts, r.Method),
		}
		for _, name := range r.Names {
			route.Names = append(route.Names, gocode.Quote(name))
		}
		v.Routes = append(v.Routes, route)
	}
	return v
}

// routerMethod is the router's method for an HTTP method: Get for GET.
func routerMethod(method string) string {
	return strings.ToUpper(method[:1]) + strings.ToLower(method[1:])
}

// packages imports every package of fw that the template name writes as .Packages.<name> into
// the file of s, and returns their names there by the name each package has on its own. The
// override text extra counts too.
func packages(fw framework.Framework, s *gocode.Scope, name, extra string) map[string]string {
	out := map[string]string{}
	for _, imp := range fw.Imports() {
		key := gocode.ImportName(imp.Path, imp.Alias)
		if uses(fw, name, extra, ".Packages."+key) {
			out[key] = s.Import(imp)
		}
	}
	return out
}

// uses reports whether the template name of fw writes field, such as .Framework, so that a view
// imports what its template writes and nothing else. The override text extra counts too.
func uses(fw framework.Framework, name, extra, field string) bool {
	text, err := fs.ReadFile(fw.Templates(), path.Join(render.Dir, name))
	return err == nil && writes(string(text)+extra, field)
}

// writes reports whether text holds field, not as the start of a longer name.
func writes(text, field string) bool {
	for {
		i := strings.Index(text, field)
		if i < 0 {
			return false
		}
		text = text[i+len(field):]
		if text == "" || !isWordByte(text[0]) {
			return true
		}
	}
}

// isWordByte reports a byte that can go on a Go identifier: an ASCII letter, a digit or _.
func isWordByte(b byte) bool {
	return b == '_' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}
