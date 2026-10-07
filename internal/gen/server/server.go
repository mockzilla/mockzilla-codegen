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
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/beego"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/chi"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/echo"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/echov5"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/fasthttp"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/fiber"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/gin"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/goframe"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/gorillamux"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/gozero"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/hertz"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/iris"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/kratos"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server/framework/stdhttp"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// The server parts. The scaffold parts are layout's, since the layout places them.
const (
	PartService layout.PartID = "server.service"
	PartErrors  layout.PartID = "server.errors"
	PartAdapter layout.PartID = "server.adapter"
	PartRouter  layout.PartID = "server.router"
)

//go:embed templates/*.tmpl
var templates embed.FS

// The blocks of the server templates a config may override.
const (
	blockServiceHeader       = "server.service-header"
	blockRequestOptionsExtra = "server.request-options-extra"
	blockResponseDataExtra   = "server.response-data-extra"
	BlockRouterExtra         = "server.router-extra"

	blockScaffoldServiceFields = "server.scaffold.service-fields"
	blockScaffoldServiceMethod = "server.scaffold.service-method"
)

// mainTemplate is the template a framework whose server is not an http.Server gives the main
// scaffold in place of the shared one.
const mainTemplate = "scaffold-main.tmpl"

// Options are the settings of the server generator. Name is the base of the interface name.
// Scaffold flags say which scaffold files the config asks for. User is the config's
// user-context, which the overridable blocks see. RouterExtra is the router-extra override text.
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
	User               map[string]any
	RouterExtra        string
}

// Scaffold says which scaffold files are written.
type Scaffold struct {
	Service    bool
	Middleware bool
	Main       bool
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

// New returns the generator of the server parts of m, with its warnings.
func New(m *gomodel.Model, opts Options) (*Generator, []diag.Diagnostic) {
	g := &Generator{opts: opts, ops: m.Operations}
	var issues []routeIssue
	g.routes, issues = routes(m.Operations, opts.Framework)

	var diags []diag.Diagnostic
	for _, is := range issues {
		diags = append(diags, warning(is.op, diag.CodeRouteDropped, is.op.Name+" is not routed: "+is.reason))
	}
	for _, op := range m.Operations {
		if op.Spec.IsWebhook {
			continue
		}
		for _, c := range op.Bodies {
			if bodyKind(c) == bodyNone {
				diags = append(diags, warning(op, diag.CodeServerBodyUnread, op.Name+" takes "+c.MediaType+", which the server does not decode into "+
					gocode.Text(gomodel.Elem(operation.BodyType(c)))+"; read it from RawRequest"))
			}
		}
		for _, r := range op.Responses {
			for _, c := range r.Contents {
				if !isWritable(c) {
					diags = append(diags, warning(op, diag.CodeServerBodyUnwritable, op.Name+" answers "+r.Status+" as "+c.MediaType+
						", which the server cannot write "+gocode.Text(gomodel.Elem(operation.BodyType(c)))+" as; set Body to a string, []byte or runtime.File"))
				}
			}
		}
	}
	return g, append(diags, rejectWarnings(m.Operations)...)
}

// Framework is the framework the router is generated for.
func (g *Generator) Framework() framework.Framework {
	return g.opts.Framework
}

// Interface is the name of the service interface.
func (g *Generator) Interface() string {
	return g.opts.Namer.Interface(g.opts.Name)
}

// Routes lists the operations the router registers.
func (g *Generator) Routes() []framework.Route {
	return g.routes
}

// Parts returns the server parts with the parts each refers to.
func (g *Generator) Parts() []layout.Part {
	var types []gomodel.Type
	for _, op := range g.ops {
		types = append(types, operationTypes(op)...)
	}
	used := operation.PartsOf(types)

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
		mainUses := []layout.PartID{PartAdapter, PartRouter, layout.PartScaffoldService}
		if g.opts.Scaffold.Middleware {
			mainUses = append(mainUses, layout.PartScaffoldMiddleware)
		}
		parts = append(parts, layout.Part{ID: layout.PartScaffoldMain, Uses: mainUses, Package: "main"})
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

// Needs names, for each block of a scaffold the config does not write, the key it has to set.
func (g *Generator) Needs() map[string]string {
	if g.opts.Scaffold.Service {
		return nil
	}
	return map[string]string{blockScaffoldServiceFields: "server.scaffold.service", blockScaffoldServiceMethod: "server.scaffold.service"}
}

// Frameworks lists the frameworks a router can be generated for, by name.
func Frameworks() map[string]framework.Framework {
	return map[string]framework.Framework{
		"beego":       beego.Framework{},
		"chi":         chi.Framework{},
		"echo":        echo.Framework{},
		"echo-v5":     echov5.Framework{},
		"fasthttp":    fasthttp.Framework{},
		"fiber":       fiber.Framework{},
		"gin":         gin.Framework{},
		"goframe":     goframe.Framework{},
		"gorilla-mux": gorillamux.Framework{},
		"go-zero":     gozero.Framework{},
		"hertz":       hertz.Framework{},
		"iris":        iris.Framework{},
		"kratos":      kratos.Framework{},
		"std-http":    stdhttp.Framework{},
	}
}

// Templates are the server template set and the framework's, which holds the router and, for a
// framework that serves in its own way, the main scaffold.
func Templates(fw framework.Framework) []render.Set {
	own := map[layout.PartID]string{PartRouter: "router.tmpl"}
	if ownsMain(fw) {
		own[layout.PartScaffoldMain] = mainTemplate
	}
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
			Blocks: []string{blockServiceHeader, blockRequestOptionsExtra, blockResponseDataExtra, blockScaffoldServiceFields, blockScaffoldServiceMethod},
		},
		{Name: fw.Name(), FS: fw.Templates(), Parts: own, Blocks: []string{BlockRouterExtra}},
	}
}

// Blocks lists the blocks of the server templates a config may override, whatever the framework.
func Blocks() []string {
	return []string{blockServiceHeader, blockRequestOptionsExtra, blockResponseDataExtra, blockScaffoldServiceFields, blockScaffoldServiceMethod, BlockRouterExtra}
}

// ownsMain reports whether fw brings the template of the main scaffold.
func ownsMain(fw framework.Framework) bool {
	_, err := fs.Stat(fw.Templates(), path.Join(render.Dir, mainTemplate))
	return err == nil
}

// routes lists the operations the router serves, without those the framework rejects.
func routes(ops []*gomodel.Operation, fw framework.Framework) ([]framework.Route, []routeIssue) {
	var out []framework.Route
	var issues []routeIssue
	for _, op := range ops {
		if op.Spec.IsWebhook {
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

func warning(op *gomodel.Operation, code, message string) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.Warning,
		Code:     code,
		Pointer:  op.Spec.Origin.Pointer,
		Origin:   diag.Origin{File: op.Spec.Origin.File, Line: op.Spec.Origin.Line, Col: op.Spec.Origin.Col},
		Message:  message,
	}
}

// isWritable reports a body httpserver.Write encodes under its media type.
func isWritable(c gomodel.Content) bool {
	mediaType := operation.BaseMediaType(c.MediaType)
	base := gomodel.Elem(operation.BodyType(c))
	under := gomodel.Underlying(base)
	switch {
	case under == stringType, under == bytesType, under == fileType, under == anyType:
		return true
	case runtime.IsJSON(mediaType), runtime.IsSequential(mediaType), strings.Contains(mediaType, "*"), mediaType == "application/x-www-form-urlencoded":
		return true
	case mediaType == "multipart/form-data":
		return gomodel.StructDecl(base) != nil
	case strings.HasPrefix(mediaType, "text/"):
		_, isBuiltin := under.(gomodel.Builtin)
		_, isQualified := under.(gomodel.Qualified)
		return isBuiltin || isQualified
	}
	return false
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
			out = append(out, c.Type, c.Item)
		}
		if r.Headers != nil {
			out = append(out, gomodel.DeclRef{Decl: r.Headers})
		}
	}
	return out
}
