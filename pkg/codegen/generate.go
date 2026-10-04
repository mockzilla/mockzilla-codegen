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
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/client"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/mcp"
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
	cfg     *config.Config
	opts    options
	diags   diag.Collector
	namer   *naming.Namer
	m       *gomodel.Model
	gen     *models.Generator
	srv     *server.Generator
	cl      *client.Generator
	mc      *mcp.Generator
	lay     *layout.Layout
	api     *API
	extras  map[layout.PartID]string
	engine  *render.Engine
	files   []File
	imports map[layout.PartID][]string
	named   map[string]bool
}

// Generate reads the spec cfg names, or the one WithSpec gives, and returns the files to write.
// cfg must come from config.Load or config.Parse, which fill in the defaults.
func Generate(ctx context.Context, cfg *config.Config, opts ...Option) (*Result, error) {
	g := &generation{cfg: cfg, opts: newOptions(opts)}
	steps := []func() error{
		cfg.Validate,
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

	var built []diag.Diagnostic
	g.m, built = gomodel.Build(doc, gomodel.OptionsFrom(g.cfg))
	g.diags.Append(built...)
	g.namer = naming.New(g.cfg.Naming.Initialisms)
	g.gen = models.New(g.m)
	if c := g.cfg.Client; c != nil {
		var clDiags []diag.Diagnostic
		g.cl, clDiags = client.New(g.m, client.Options{
			Name:         cmp.Or(c.Name, "Client"),
			Namer:        g.namer,
			Timeout:      time.Duration(*cmp.Or(c.Timeout, new(config.Duration(3*time.Second)))),
			HasEnvelopes: c.WithResponse,
			HasStreams:   c.Streaming,
			User:         g.cfg.UserContext,
		})
		g.diags.Append(clDiags...)
	}
	if m := g.cfg.MCP; m != nil {
		var mcDiags []diag.Diagnostic
		g.mc, mcDiags = mcp.New(g.m, mcp.Options{
			Client:      cmp.Or(g.cfg.Client.Name, "Client"),
			Namer:       g.namer,
			DefaultSkip: m.DefaultSkip,
			User:        g.cfg.UserContext,
		})
		g.diags.Append(mcDiags...)
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
		User:               g.cfg.UserContext,
	}, nil
}

// place lays every part out, the extra files included, and describes the result for their
// templates.
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
	if g.mc != nil {
		parts = append(parts, g.mc.Parts()...)
	}
	for _, rel := range slices.Sorted(maps.Keys(g.cfg.ExtraFiles)) {
		parts = append(parts, layout.Part{ID: layout.PartID(rel)})
	}

	g.lay, err = layout.Plan(g.cfg, parts, mod)
	if err != nil {
		return err
	}
	if err = g.checkNames(); err != nil {
		return err
	}
	if len(g.cfg.ExtraFiles) > 0 {
		g.api = describe(g)
	}
	return nil
}

// checkNames fails when the service and the client have one name in one package, where both
// declare the same interface.
func (g *generation) checkNames() error {
	if g.srv == nil || g.cl == nil || g.srv.Interface() != g.cl.Interface() {
		return nil
	}
	service, ops := g.lay.FileOf(server.PartService), g.lay.FileOf(client.PartOperations)
	if filepath.Dir(service.Path) != filepath.Dir(ops.Path) {
		return nil
	}
	return fmt.Errorf("%w: server.name and client.name are both %q, so package %s declares %s twice; name one of them differently", ErrNameClash, g.cfg.Server.Name, service.Package, g.srv.Interface())
}

// load loads the templates, the block overrides and the extra files of the config. The blocks of
// the server are named also when the config asks for none, so that an override of one says what
// it needs.
func (g *generation) load() error {
	sets := []render.Set{models.Templates()}
	needs := make(map[string]string)
	if g.srv != nil {
		sets = append(sets, server.Templates(g.srv.Framework())...)
		maps.Copy(needs, g.srv.Needs())
	} else {
		for _, block := range server.Blocks() {
			needs[block] = "a server block"
		}
	}
	if g.cl != nil {
		sets = append(sets, client.Templates())
	}
	if g.mc != nil {
		sets = append(sets, mcp.Templates())
	}
	overrides, err := g.templates()
	if err != nil {
		return err
	}
	g.extras = make(map[layout.PartID]string, len(g.cfg.ExtraFiles))
	for _, rel := range slices.Sorted(maps.Keys(g.cfg.ExtraFiles)) {
		if g.extras[layout.PartID(rel)], err = g.text(fmt.Sprintf("extra-files[%q]", rel), g.cfg.ExtraFiles[rel]); err != nil {
			return err
		}
	}

	isFormat := g.cfg.Output.Format == nil || *g.cfg.Output.Format
	g.engine, err = render.New(sets, render.Options{Templates: overrides, Needs: needs, Format: isFormat})
	return err
}

// templates returns the text of every block override of the config.
func (g *generation) templates() (map[string]string, error) {
	out := make(map[string]string, len(g.cfg.Templates))
	for _, name := range slices.Sorted(maps.Keys(g.cfg.Templates)) {
		text, err := g.text("templates."+name, g.cfg.Templates[name])
		if err != nil {
			return nil, err
		}
		out[name] = text
	}
	return out, nil
}

// text is the text of t, the template under config key key: the one it holds, or that of the
// file it names.
func (g *generation) text(key string, t config.Template) (string, error) {
	if t.File == "" {
		return t.Text, nil
	}

	data, err := os.ReadFile(g.cfg.Resolve(t.File))
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrTemplateFile, key, err)
	}
	return string(data), nil
}

// render writes every file of the layout, then checks the imports the parts brought along: an
// extra file can use a folder that imports its own, which the layout could not know.
func (g *generation) render() error {
	g.imports = make(map[layout.PartID][]string)
	g.named = make(map[string]bool)
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

	g.reportUnusedImports()
	return g.lay.CheckImports(g.imports)
}

// file renders f with the imports of the config on offer. An offer no part took, which cost
// another import its name, is taken back and f rendered again: an unused entry changes nothing.
func (g *generation) file(f *layout.File) ([]byte, error) {
	offers := g.cfg.Imports
	for {
		s := gocode.NewScope(f, g.lay)
		for _, imp := range offers {
			s.Imports.Offer(imp.Package, imp.Alias)
		}
		data, err := g.parts(f, s)
		if err != nil {
			return nil, err
		}

		idle := s.Imports.Idle()
		if len(idle) > 0 {
			// An import under _ is no offer: it stays when the path is also listed under a name.
			offers = slices.DeleteFunc(slices.Clone(offers), func(imp config.Import) bool {
				return imp.Name() != "" && slices.Contains(idle, imp.Package)
			})
			continue
		}

		for _, imp := range g.cfg.Imports {
			if _, ok := s.Imports.Name(imp.Package); ok {
				g.named[imp.Package] = true
			}
		}
		data.Imports = s.Imports.Decl()
		data.Guard = s.RuntimeGuard()
		return g.engine.RenderFile(data)
	}
}

// parts renders the parts of f. A part that names an import on offer takes it.
func (g *generation) parts(f *layout.File, s *gocode.Scope) (render.FileData, error) {
	data := render.FileData{Header: g.cfg.Header, Package: f.Package}
	for _, part := range f.Parts {
		out, err := g.part(part, s)
		if err != nil {
			return render.FileData{}, err
		}
		data.Parts = append(data.Parts, string(out))
		s.Imports.Take(out)
		g.imports[part] = s.Imports.Paths()
	}
	return data, nil
}

// part renders one part of the file of s: an extra file from its template, a built-in one from
// its view.
func (g *generation) part(id layout.PartID, s *gocode.Scope) ([]byte, error) {
	if text, ok := g.extras[id]; ok {
		return renderExtra(id, text, g.api, s)
	}
	return g.engine.RenderPart(id, g.view(id, s))
}

// view is the template data of part: the server generator's for server parts, the client
// generator's for client parts, the MCP generator's for MCP parts, else the models'.
func (g *generation) view(part layout.PartID, s *gocode.Scope) any {
	switch {
	case strings.HasPrefix(string(part), "server."):
		return g.srv.View(part, s)
	case strings.HasPrefix(string(part), "client."):
		return g.cl.View(part, s)
	case strings.HasPrefix(string(part), "mcp."):
		return g.mc.View(part, s)
	}
	return g.gen.View(part, s)
}

// reportUnusedImports warns about every import of the config that no file has under its name.
// One under _ is in every file.
func (g *generation) reportUnusedImports() {
	for i, imp := range g.cfg.Imports {
		if imp.Name() == "" || g.named[imp.Package] {
			continue
		}
		g.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeImportUnused,
			Message:  fmt.Sprintf("imports[%d]: no generated file refers to %s, so %s is not imported", i, imp.Name(), imp.Package),
		})
	}
}
