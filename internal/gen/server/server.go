// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package server turns the operations of the Go model into a server: the service contract a
// service implements, the HTTP adapter that calls it, the router of one framework, and the
// scaffold files a project starts from.
package server

import (
	"embed"
	"slices"
	"strings"
	"time"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/chi"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/stdhttp"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
)

// The server parts. The scaffold parts are layout's, since the layout places them.
const (
	PartService layout.PartID = "server.service"
	PartErrors  layout.PartID = "server.errors"
	PartAdapter layout.PartID = "server.adapter"
	PartRouter  layout.PartID = "server.router"
)

//go:embed *.tmpl
var templates embed.FS

// routerMethods are the HTTP methods every router registers.
var routerMethods = []string{"GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE"}

// The blocks of the server templates a config may override.
const (
	BlockServiceHeader       = "server.service-header"
	BlockRequestOptionsExtra = "server.request-options-extra"
	BlockResponseDataExtra   = "server.response-data-extra"
	BlockRouterExtra         = "server.router-extra"
)

// Options are the settings of the server generator. Name is the base of the interface name.
// Scaffold flags say which scaffold files the config asks for. ExtraFields are added to the
// request options of every operation; User is the config's user-context, which the overridable
// blocks see.
type Options struct {
	Name               string
	Namer              *naming.Namer
	Framework          framework.Framework
	ValidateRequest    bool
	ValidateResponse   bool
	MultipartMaxMemory int64
	Scaffold           Scaffold
	Port               int
	Timeout            time.Duration
	ExtraFields        []ExtraField
	User               map[string]any
}

// Scaffold says which scaffold files are written.
type Scaffold struct {
	Service    bool
	Middleware bool
	Main       bool
}

// ExtraField is a field a plugin adds to every request options struct. Type is the type as the
// package of Import writes it; Import is empty for a type that needs none.
type ExtraField struct {
	Name   string
	Type   string
	Doc    string
	Import gomodel.Import
}

// Generator builds the template data of every server part.
type Generator struct {
	opts   Options
	ops    []*gomodel.Operation
	routes []framework.Route
}

// routeIssue is an operation the router cannot serve, and why.
type routeIssue struct {
	op     *gomodel.Operation
	reason string
}

// New returns the generator of the server parts of m, and a warning for each operation the
// router cannot serve.
func New(m *gomodel.Model, opts Options) (*Generator, []diag.Diagnostic) {
	g := &Generator{opts: opts, ops: m.Operations}
	var issues []routeIssue
	g.routes, issues = routes(m.Operations, opts.Framework)

	var diags []diag.Diagnostic
	for _, is := range issues {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeRouteDropped,
			Pointer:  is.op.Spec.Origin.Pointer,
			Origin:   diag.Origin{File: is.op.Spec.Origin.File, Line: is.op.Spec.Origin.Line, Col: is.op.Spec.Origin.Col},
			Message:  is.op.Name + " is not routed: " + is.reason,
		})
	}
	return g, diags
}

// Framework is the framework the router is generated for.
func (g *Generator) Framework() framework.Framework {
	return g.opts.Framework
}

// Routes lists the operations the router registers.
func (g *Generator) Routes() []framework.Route {
	return g.routes
}

// Parts returns the server parts with the parts each refers to.
func (g *Generator) Parts() []layout.Part {
	var used []layout.PartID
	for _, op := range g.ops {
		for _, t := range operationTypes(op) {
			for _, d := range decls(t) {
				if id := layout.PartID(d.Part); !slices.Contains(used, id) {
					used = append(used, id)
				}
			}
		}
	}
	slices.Sort(used)

	parts := []layout.Part{
		{ID: PartService, Uses: used},
		{ID: PartErrors},
		{ID: PartAdapter, Uses: append([]layout.PartID{PartService}, used...)},
		{ID: PartRouter, Uses: []layout.PartID{PartService, PartAdapter}},
	}
	if g.opts.Scaffold.Service {
		parts = append(parts, layout.Part{ID: layout.PartScaffoldService, Uses: append([]layout.PartID{PartService}, used...)})
	}
	if g.opts.Scaffold.Middleware {
		parts = append(parts, layout.Part{ID: layout.PartScaffoldMiddleware})
	}
	if g.opts.Scaffold.Main {
		uses := []layout.PartID{PartAdapter, PartRouter, layout.PartScaffoldService}
		if g.opts.Scaffold.Middleware {
			uses = append(uses, layout.PartScaffoldMiddleware)
		}
		parts = append(parts, layout.Part{ID: layout.PartScaffoldMain, Uses: uses, Package: "main"})
	}
	return parts
}

// View returns the data of part, with names written as the file of s spells them.
func (g *Generator) View(part layout.PartID, s *gocode.Scope) any {
	switch part {
	case PartErrors:
		return errorsView(s)
	case PartAdapter:
		return adapterView(g, s)
	case PartRouter:
		return routerView(g, s)
	case layout.PartScaffoldService:
		return scaffoldServiceView(g, s)
	case layout.PartScaffoldMiddleware:
		return scaffoldMiddlewareView(s)
	case layout.PartScaffoldMain:
		return scaffoldMainView(g, s)
	default:
		return serviceView(g, s)
	}
}

// Frameworks lists the frameworks a router can be generated for, by name.
func Frameworks() map[string]framework.Framework {
	return map[string]framework.Framework{"chi": chi.Framework{}, "std-http": stdhttp.Framework{}}
}

// Templates are the server template set and the framework's, which holds the router.
func Templates(fw framework.Framework) []render.Set {
	return []render.Set{
		{
			Name: "server",
			FS:   templates,
			Parts: map[layout.PartID]string{
				PartService:                   "service.tmpl",
				PartErrors:                    "errors.tmpl",
				PartAdapter:                   "adapter.tmpl",
				layout.PartScaffoldService:    "scaffold-service.tmpl",
				layout.PartScaffoldMiddleware: "scaffold-middleware.tmpl",
				layout.PartScaffoldMain:       "scaffold-main.tmpl",
			},
			Blocks: []string{BlockServiceHeader, BlockRequestOptionsExtra, BlockResponseDataExtra},
		},
		{Name: fw.Name(), FS: fw.Templates(), Parts: map[layout.PartID]string{PartRouter: "router.tmpl"}, Blocks: []string{BlockRouterExtra}},
	}
}

// ReservedField reports whether name is a field the request options declare themselves, so an
// extra field cannot take it: a parameter group, a body field, RawRequest or the Validate method.
func ReservedField(name string) bool {
	return name == "RawRequest" || name == "Validate" || strings.HasPrefix(name, "Body") || operation.IsGroupField(name)
}

// routes lists the operations the router serves, without those the framework rejects.
func routes(ops []*gomodel.Operation, fw framework.Framework) ([]framework.Route, []routeIssue) {
	var out []framework.Route
	var issues []routeIssue
	for _, op := range ops {
		if op.Spec.IsWebhook {
			continue
		}
		if !slices.Contains(routerMethods, op.Spec.Method) {
			issues = append(issues, routeIssue{op: op, reason: "the router does not take the method " + op.Spec.Method})
			continue
		}
		pattern, err := fw.RoutePattern(op.Spec.Method, op.Spec.Path)
		if err != nil {
			issues = append(issues, routeIssue{op: op, reason: err.Error()})
			continue
		}
		out = append(out, framework.Route{Operation: op.Name, Method: op.Spec.Method, Path: op.Spec.Path, Pattern: pattern})
	}

	kept, dropped := fw.Conflicts(out)
	for _, c := range dropped {
		i := slices.IndexFunc(ops, func(op *gomodel.Operation) bool { return op.Name == c.Route.Operation })
		issues = append(issues, routeIssue{op: ops[i], reason: c.Reason})
	}
	return kept, issues
}

// operationTypes lists the types an operation's contract names.
func operationTypes(op *gomodel.Operation) []gomodel.Type {
	var out []gomodel.Type
	for _, p := range op.Params {
		out = append(out, gomodel.DeclRef{Decl: p.Decl})
	}
	for _, c := range op.Bodies {
		out = append(out, c.Type)
	}
	for _, r := range op.Responses {
		for _, c := range r.Contents {
			out = append(out, c.Type)
		}
		if r.Headers != nil {
			out = append(out, gomodel.DeclRef{Decl: r.Headers})
		}
	}
	return out
}

// decls returns the declarations a type refers to.
func decls(t gomodel.Type) []*gomodel.Decl {
	switch t := t.(type) {
	case gomodel.DeclRef:
		return []*gomodel.Decl{t.Decl}
	case gomodel.Pointer:
		return decls(t.Elem)
	case gomodel.Slice:
		return decls(t.Elem)
	case gomodel.Map:
		return slices.Concat(decls(t.Key), decls(t.Elem))
	}
	return nil
}
