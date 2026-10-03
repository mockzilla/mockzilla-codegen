// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of the three scaffolds: the service stub, the middleware and main.

package server

import (
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
)

// middlewares are the middleware functions of the scaffold, in the order main wraps them.
var middlewares = []string{"RequestIDMiddleware", "RecoverMiddleware", "LoggingMiddleware", "CORSMiddleware"}

// ScaffoldServiceView is the data of the service scaffold: the struct named Name, which
// implements Interface. User is the config's user-context, which the blocks see.
type ScaffoldServiceView struct {
	Name       string
	Interface  string
	Context    string
	Errors     string
	User       map[string]any
	Operations []ScaffoldOperationView
}

// ScaffoldOperationView is one stub method.
type ScaffoldOperationView struct {
	Name    string
	Method  string
	Path    string
	Options string
	Data    string
	User    map[string]any
}

// ScaffoldMiddlewareView is the data of the middleware scaffold: the packages it imports.
type ScaffoldMiddlewareView struct {
	HTTP  string
	Slog  string
	Time  string
	Rand  string
	Debug string
}

// ScaffoldMainView is the data of the main scaffold. Middleware lists the middleware
// expressions main passes, empty without the middleware scaffold; Timeout is a duration
// expression. HTTP is set for the shared template, which serves with an http.Server; Framework
// and Packages for the template of a framework that serves in its own way, as in the router's
// view.
type ScaffoldMainView struct {
	Context        string
	HTTP           string
	Framework      string
	Packages       map[string]string
	Slog           string
	OS             string
	Signal         string
	Syscall        string
	NewRouter      string
	WithRouter     string
	NewService     string
	WithMiddleware string
	Middleware     []string
	Port           int
	Timeout        string
}

func scaffoldServiceView(g *Generator, s *gocode.Scope) *ScaffoldServiceView {
	v := &ScaffoldServiceView{
		Name:      g.opts.Name,
		Interface: s.Symbol(PartService, g.Interface()),
		Errors:    s.Import(gomodel.Import{Path: "errors"}),
		User:      g.opts.User,
	}
	if len(g.ops) > 0 {
		v.Context = s.Import(gomodel.Import{Path: "context"})
	}
	for _, op := range g.ops {
		v.Operations = append(v.Operations, ScaffoldOperationView{
			Name:    op.Name,
			Method:  op.Spec.Method,
			Path:    op.Spec.Path,
			Options: s.Symbol(PartService, g.opts.Namer.ServiceRequestOptions(op.Name)),
			Data:    s.Symbol(PartService, g.opts.Namer.ResponseData(op.Name)),
			User:    g.opts.User,
		})
	}
	return v
}

func scaffoldMiddlewareView(s *gocode.Scope) *ScaffoldMiddlewareView {
	return &ScaffoldMiddlewareView{
		HTTP:  s.Import(gomodel.Import{Path: "net/http"}),
		Slog:  s.Import(gomodel.Import{Path: "log/slog"}),
		Time:  s.Import(gomodel.Import{Path: "time"}),
		Rand:  s.Import(gomodel.Import{Path: "crypto/rand"}),
		Debug: s.Import(gomodel.Import{Path: "runtime/debug"}),
	}
}

func scaffoldMainView(g *Generator, s *gocode.Scope) *ScaffoldMainView {
	timePkg := s.Import(gomodel.Import{Path: "time"})
	v := &ScaffoldMainView{
		Context:        s.Import(gomodel.Import{Path: "context"}),
		Slog:           s.Import(gomodel.Import{Path: "log/slog"}),
		OS:             s.Import(gomodel.Import{Path: "os"}),
		Signal:         s.Import(gomodel.Import{Path: "os/signal"}),
		Syscall:        s.Import(gomodel.Import{Path: "syscall"}),
		NewRouter:      s.Symbol(PartRouter, "NewRouter"),
		WithRouter:     s.Symbol(PartRouter, "WithRouter"),
		NewService:     s.Symbol(layout.PartScaffoldService, "New"+g.opts.Name),
		WithMiddleware: s.Symbol(PartAdapter, "WithMiddleware"),
		Port:           g.opts.Port,
		Timeout:        gocode.Duration(g.opts.Timeout, timePkg),
	}
	if fw := g.opts.Framework; ownsMain(fw) {
		if uses(fw, mainTemplate, ".Framework") {
			v.Framework = s.Import(fw.Imports()[0])
		}
		v.Packages = packages(fw, s, mainTemplate)
	} else {
		v.HTTP = s.Import(gomodel.Import{Path: "net/http"})
	}
	if !g.opts.Scaffold.Middleware {
		return v
	}
	for _, name := range middlewares {
		v.Middleware = append(v.Middleware, s.Symbol(layout.PartScaffoldMiddleware, name))
	}
	timeout := s.Symbol(layout.PartScaffoldMiddleware, "TimeoutMiddleware")
	v.Middleware = append(v.Middleware, gocode.Call(timeout, v.Timeout))
	return v
}
