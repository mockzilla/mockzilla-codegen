// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
)

// RouterView is the data of the router part. Framework is the name the framework's package is
// imported under; the other names are written as the file spells them.
type RouterView struct {
	Framework  string
	Service    string
	Option     string
	Options    string
	NewOptions string
	NewAdapter string
	Routes     []RouteView
}

// RouteView registers one operation: Method as the router's method, Pattern quoted.
type RouteView struct {
	Method    string
	Pattern   string
	Operation string
}

func routerView(g *Generator, s *gocode.Scope) *RouterView {
	fw := g.opts.Framework
	v := &RouterView{
		Framework:  s.Import(fw.Imports()[0]),
		Service:    s.Symbol(PartService, g.opts.Name+"Interface"),
		Option:     s.Symbol(PartAdapter, "ServerOption"),
		Options:    s.Symbol(PartAdapter, "ServerOptions"),
		NewOptions: s.Symbol(PartAdapter, "NewServerOptions"),
		NewAdapter: s.Symbol(PartAdapter, "NewHTTPAdapter"),
	}
	for _, r := range g.routes {
		v.Routes = append(v.Routes, RouteView{Method: routerMethod(r.Method), Pattern: gocode.Quote(r.Pattern), Operation: r.Operation})
	}
	return v
}

// routerMethod is the router's method for an HTTP method: Get for GET.
func routerMethod(method string) string {
	return strings.ToUpper(method[:1]) + strings.ToLower(method[1:])
}
