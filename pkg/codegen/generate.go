// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"cmp"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/models"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/prepare"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

type FileKind int

const (
	// FileGenerated is rewritten on every run.
	FileGenerated FileKind = iota
	// FileScaffold is a starter file, written only when missing unless told to overwrite.
	FileScaffold
)

// Result is what Generate made: every file with its content, and what the spec and generation
// reported, sorted by position.
type Result struct {
	Files       []File
	Diagnostics []Diagnostic
}

// File is one generated file. Path is resolved against the config folder; Parts lists the parts it
// holds, such as models.types.
type File struct {
	Path    string
	Package string
	Parts   []string
	Kind    FileKind
	Content []byte
}

// generation is one Generate run.
type generation struct {
	cfg    *config.Config
	opts   options
	diags  diag.Collector
	gen    *models.Generator
	srv    *server.Generator
	lay    *layout.Layout
	engine *render.Engine
	files  []File
}

// Generate reads the spec cfg names, or the one WithSpec gives, and returns the files to write.
// cfg must come from config.Load or config.Parse, which fill in the defaults.
func Generate(ctx context.Context, cfg *config.Config, opts ...Option) (*Result, error) {
	g := &generation{cfg: cfg, opts: newOptions(opts)}
	steps := []func() error{
		cfg.Validate,
		func() error { return g.model(ctx) },
		g.plan,
		g.render,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return &Result{Files: g.files, Diagnostics: diagnostics(g.diags.List())}, nil
}

// model prepares and parses the spec, then builds the Go model and its generator.
func (g *generation) model(ctx context.Context) error {
	prepared, err := prepare.Run(ctx, g.opts.provider, prepare.Input{Spec: g.opts.spec, Config: g.cfg})
	if err != nil {
		return err
	}
	g.diags.Append(prepared.Diagnostics...)

	doc, parsed, err := g.opts.provider.Parse(ctx, prepared.Bytes, provider.ParseOptions{File: prepared.File, Positions: prepared.Positions})
	if err != nil {
		return err
	}
	g.diags.Append(parsed...)

	m, built := gomodel.Build(doc, gomodel.OptionsFrom(g.cfg))
	g.diags.Append(built...)
	g.gen = models.New(m)
	if g.cfg.Server == nil {
		return nil
	}

	opts, err := serverOptions(g.cfg)
	if err != nil {
		return err
	}
	var srvDiags []diag.Diagnostic
	g.srv, srvDiags = server.New(m, opts)
	g.diags.Append(srvDiags...)
	return nil
}

// serverOptions reads the server block of cfg. The framework it names has to be one the generator
// knows.
func serverOptions(cfg *config.Config) (server.Options, error) {
	s := cfg.Server
	fw, ok := server.Frameworks()[s.Framework]
	if !ok {
		return server.Options{}, fmt.Errorf("%w: %s", ErrFramework, s.Framework)
	}
	return server.Options{
		Name:               cmp.Or(s.Name, "Service"),
		Namer:              naming.New(cfg.Naming.Initialisms),
		Framework:          fw,
		ValidateRequest:    s.Validation.Request,
		ValidateResponse:   s.Validation.Response,
		MultipartMaxMemory: int64(s.MultipartMaxMemory),
		Scaffold:           server.Scaffold{Service: s.Scaffold.Service != "", Middleware: s.Scaffold.Middleware != "", Main: s.Scaffold.Main != ""},
		Port:               s.Scaffold.Port,
		Timeout:            time.Duration(s.Scaffold.Timeout),
	}, nil
}

// plan places the parts in files and loads the templates.
func (g *generation) plan() error {
	mod, err := layout.FindModule(g.cfg.Resolve("."), g.cfg.Output.Module)
	if err != nil {
		return err
	}
	parts, sets := g.gen.Parts(), []render.Set{models.Templates()}
	if g.srv != nil {
		parts = append(parts, g.srv.Parts()...)
		sets = append(sets, server.Templates(g.srv.Framework())...)
	}
	if g.lay, err = layout.Plan(g.cfg, parts, mod); err != nil {
		return err
	}

	isFormat := g.cfg.Output.Format == nil || *g.cfg.Output.Format
	g.engine, err = render.New(sets, render.Options{Templates: g.cfg.Templates, Format: isFormat})
	return err
}

// render writes every file of the layout.
func (g *generation) render() error {
	for _, f := range g.lay.Files {
		content, err := g.file(f)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Rel, err)
		}
		parts := make([]string, len(f.Parts))
		for i, p := range f.Parts {
			parts[i] = string(p)
		}
		g.files = append(g.files, File{Path: f.Path, Package: f.Package, Parts: parts, Kind: FileKind(f.Kind), Content: content})
	}
	return nil
}

// view is the template data of part: the server generator's for server parts, else the models'.
func (g *generation) view(part layout.PartID, s *gocode.Scope) any {
	if strings.HasPrefix(string(part), "server.") {
		return g.srv.View(part, s)
	}
	return g.gen.View(part, s)
}

func (g *generation) file(f *layout.File) ([]byte, error) {
	s := gocode.NewScope(f, g.lay)
	data := render.FileData{Header: g.cfg.Header, Package: f.Package}
	for _, part := range f.Parts {
		out, err := g.engine.RenderPart(part, g.view(part, s))
		if err != nil {
			return nil, err
		}
		data.Parts = append(data.Parts, string(out))
	}

	data.Imports = s.Imports.Decl()
	data.Guard = s.RuntimeGuard()
	return g.engine.RenderFile(data)
}
