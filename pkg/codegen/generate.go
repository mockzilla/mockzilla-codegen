// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/client"
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

// templateExt marks a templates value that names a file instead of holding the text.
const templateExt = ".tmpl"

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
	cfg      *config.Config
	opts     options
	diags    diag.Collector
	reserved reservations
	namer    *naming.Namer
	m        *gomodel.Model
	gen      *models.Generator
	srv      *server.Generator
	cl       *client.Generator
	sources  map[layout.PartID]source
	lay      *layout.Layout
	engine   *render.Engine
	files    []File
}

// Generate reads the spec cfg names, or the one WithSpec gives, and returns the files to write.
// cfg must come from config.Load or config.Parse, which fill in the defaults.
func Generate(ctx context.Context, cfg *config.Config, opts ...Option) (*Result, error) {
	g := &generation{cfg: cfg, opts: newOptions(opts), sources: make(map[layout.PartID]source)}
	steps := []func() error{
		cfg.Validate,
		g.reserve,
		func() error { return g.model(ctx) },
		g.place,
		g.load,
		g.render,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return &Result{Files: g.files, Diagnostics: diagnostics(g.diags.List())}, nil
}

// model prepares and parses the spec, then builds the Go model and its generators.
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

	opts := gomodel.OptionsFrom(g.cfg)
	opts.Reserved = append(opts.Reserved, g.reserved.idents...)
	var built []diag.Diagnostic
	g.m, built = gomodel.Build(doc, opts)
	g.diags.Append(built...)
	g.namer = naming.New(g.cfg.Naming.Initialisms)
	g.gen = models.New(g.m)
	if c := g.cfg.Client; c != nil {
		g.cl = client.New(g.m, client.Options{
			Name:         cmp.Or(c.Name, "Client"),
			Namer:        g.namer,
			Timeout:      time.Duration(c.Timeout),
			HasEnvelopes: c.WithResponse,
			User:         g.cfg.UserContext,
		})
	}
	if g.cfg.Server == nil {
		return nil
	}

	srvOpts, err := g.serverOptions()
	if err != nil {
		return err
	}
	var srvDiags []diag.Diagnostic
	g.srv, srvDiags = server.New(g.m, srvOpts)
	g.diags.Append(srvDiags...)
	return nil
}

// serverOptions reads the server block of the config. The framework it names has to be one the
// generator knows.
func (g *generation) serverOptions() (server.Options, error) {
	s := g.cfg.Server
	fw, ok := server.Frameworks()[s.Framework]
	if !ok {
		return server.Options{}, fmt.Errorf("%w: %s", ErrFramework, s.Framework)
	}
	return server.Options{
		Name:               cmp.Or(s.Name, "Service"),
		Namer:              g.namer,
		Framework:          fw,
		ValidateRequest:    s.Validation.Request,
		ValidateResponse:   s.Validation.Response,
		MultipartMaxMemory: int64(s.MultipartMaxMemory),
		Scaffold:           server.Scaffold{Service: s.Scaffold.Service != "", Middleware: s.Scaffold.Middleware != "", Main: s.Scaffold.Main != ""},
		Port:               s.Scaffold.Port,
		Timeout:            time.Duration(s.Scaffold.Timeout),
		ExtraFields:        g.reserved.fields,
		User:               g.cfg.UserContext,
	}, nil
}

// place lays the built-in parts out, lets the plugins contribute theirs against that draft, then
// lays every part out.
func (g *generation) place() error {
	mod, err := layout.FindModule(g.cfg.Resolve("."), g.cfg.Output.Module)
	if err != nil {
		return err
	}

	parts := g.gen.Parts()
	if g.srv != nil {
		parts = append(parts, g.srv.Parts()...)
	}
	if g.cl != nil {
		parts = append(parts, g.cl.Parts()...)
	}
	if len(g.opts.plugins) > 0 {
		draft, draftErr := layout.Draft(g.cfg, parts, mod)
		if draftErr != nil {
			return draftErr
		}
		added, addErr := g.contribute(draft)
		if addErr != nil {
			return addErr
		}
		parts = append(parts, added...)
	}

	g.lay, err = layout.Plan(g.cfg, parts, mod)
	return err
}

// load loads the templates and the block overrides of the config.
func (g *generation) load() error {
	sets := []render.Set{models.Templates()}
	if g.srv != nil {
		sets = append(sets, server.Templates(g.srv.Framework())...)
	}
	if g.cl != nil {
		sets = append(sets, client.Templates())
	}
	overrides, err := g.templates()
	if err != nil {
		return err
	}

	isFormat := g.cfg.Output.Format == nil || *g.cfg.Output.Format
	g.engine, err = render.New(sets, render.Options{Templates: overrides, Format: isFormat})
	return err
}

// templates returns the block overrides of the config, with a value that names a .tmpl file
// replaced by that file's text.
func (g *generation) templates() (map[string]string, error) {
	out := make(map[string]string, len(g.cfg.Templates))
	for _, name := range slices.Sorted(maps.Keys(g.cfg.Templates)) {
		text := g.cfg.Templates[name]
		if strings.HasSuffix(text, templateExt) {
			data, err := os.ReadFile(g.cfg.Resolve(text))
			if err != nil {
				return nil, fmt.Errorf("%w: templates.%s: %w", ErrTemplateFile, name, err)
			}
			text = string(data)
		}
		out[name] = text
	}
	return out, nil
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

// view is the template data of part: the server generator's for server parts, the client
// generator's for client parts, else the models'.
func (g *generation) view(part layout.PartID, s *gocode.Scope) any {
	switch {
	case strings.HasPrefix(string(part), "server."):
		return g.srv.View(part, s)
	case strings.HasPrefix(string(part), "client."):
		return g.cl.View(part, s)
	}
	return g.gen.View(part, s)
}

func (g *generation) file(f *layout.File) ([]byte, error) {
	s := gocode.NewScope(f, g.lay)
	data := render.FileData{Header: g.cfg.Header, Package: f.Package}
	for _, part := range f.Parts {
		out, err := g.part(part, s)
		if err != nil {
			return nil, err
		}
		data.Parts = append(data.Parts, string(out))
	}

	data.Imports = s.Imports.Decl()
	data.Guard = s.RuntimeGuard()
	return g.engine.RenderFile(data)
}
