// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"fmt"
	"go/token"
	"maps"
	"regexp"
	"slices"
	"text/template"

	"github.com/mockzilla/mockzilla-codegen/internal/gen/client"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/server"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
	"github.com/mockzilla/mockzilla-codegen/internal/render"
)

// segment is the form of a plugin name and of a part name.
var segment = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// scaffoldParts are the parts the scaffold kinds replace.
var scaffoldParts = map[ScaffoldKind]layout.PartID{
	ScaffoldService:    layout.PartScaffoldService,
	ScaffoldMiddleware: layout.PartScaffoldMiddleware,
	ScaffoldMain:       layout.PartScaffoldMain,
}

// reservations is what every plugin reserves together, in the order the plugins are given.
type reservations struct {
	idents []string
	fields []server.ExtraField
}

// source is a template a plugin gave: a part of its own with its data, or the replacement of a
// scaffold template, which runs on the built-in view.
type source struct {
	plugin     string
	text       string
	funcs      template.FuncMap
	imports    []Import
	data       any
	isScaffold bool
}

// reserve checks the plugins' names and collects what they reserve.
func (g *generation) reserve() error {
	var names []string
	for _, p := range g.opts.plugins {
		name := p.Name()
		switch {
		case !segment.MatchString(name):
			return fmt.Errorf("%w %q: the name must match [a-z][a-z0-9]*", ErrPlugin, name)
		case slices.Contains(names, name):
			return fmt.Errorf("%w %s: the name is used twice", ErrPlugin, name)
		}
		names = append(names, name)

		res := p.Reserve()
		g.reserved.idents = append(g.reserved.idents, res.Idents...)
		for _, f := range res.RequestOptionFields {
			if err := g.reserveField(name, f); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *generation) reserveField(plugin string, f FieldSpec) error {
	switch {
	case !token.IsIdentifier(f.Name) || !token.IsExported(f.Name):
		return fmt.Errorf("%w %s: request option field %q is no exported identifier", ErrPlugin, plugin, f.Name)
	case f.Type.Name == "":
		return fmt.Errorf("%w %s: request option field %s has no type", ErrPlugin, plugin, f.Name)
	case server.ReservedField(f.Name):
		return fmt.Errorf("%w %s: request option field %s is one the request options declare themselves", ErrPlugin, plugin, f.Name)
	case slices.ContainsFunc(g.reserved.fields, func(x server.ExtraField) bool { return x.Name == f.Name }):
		return fmt.Errorf("%w %s: request option field %s is added twice", ErrPlugin, plugin, f.Name)
	}

	g.reserved.fields = append(g.reserved.fields, server.ExtraField{
		Name:   f.Name,
		Type:   f.Type.Name,
		Doc:    f.Doc,
		Import: gomodel.Import{Path: f.Type.ImportPath, Alias: f.Type.Package},
	})
	return nil
}

// contribute shows every plugin the API, as the draft layout places it, and keeps what each
// gives. It returns the parts the plugins add.
func (g *generation) contribute(draft *layout.Layout) ([]layout.Part, error) {
	shown := g.api(draft)
	var parts []layout.Part
	for _, p := range g.opts.plugins {
		c, err := p.Contribute(shown)
		if err != nil {
			return nil, fmt.Errorf("%w %s: %w", ErrPlugin, p.Name(), err)
		}
		if c == nil {
			continue
		}

		for _, src := range c.Parts {
			id, addErr := g.addPart(p.Name(), src, c.Funcs)
			if addErr != nil {
				return nil, addErr
			}
			parts = append(parts, layout.Part{ID: id})
		}
		for _, kind := range slices.Sorted(maps.Keys(c.Scaffolds)) {
			if err = g.replaceScaffold(p.Name(), kind, source{text: c.Scaffolds[kind], funcs: c.Funcs}); err != nil {
				return nil, err
			}
		}
	}
	return parts, nil
}

func (g *generation) addPart(plugin string, src PartSource, funcs template.FuncMap) (layout.PartID, error) {
	id := layout.PartID("plugin." + plugin + "." + src.Name)
	switch _, taken := g.sources[id]; {
	case !segment.MatchString(src.Name):
		return "", fmt.Errorf("%w %s: the part name %q must match [a-z][a-z0-9]*", ErrPlugin, plugin, src.Name)
	case taken:
		return "", fmt.Errorf("%w %s: the part %s is contributed twice", ErrPlugin, plugin, src.Name)
	}

	g.sources[id] = source{plugin: plugin, text: src.Template, funcs: funcs, imports: src.Imports, data: src.Data}
	return id, nil
}

func (g *generation) replaceScaffold(plugin string, kind ScaffoldKind, src source) error {
	id, ok := scaffoldParts[kind]
	if !ok {
		return fmt.Errorf("%w %s: %d is no scaffold kind", ErrPlugin, plugin, kind)
	}
	if other, taken := g.sources[id]; taken {
		return fmt.Errorf("%w %s: the %s scaffold is already replaced by %s", ErrPlugin, plugin, kind, other.plugin)
	}

	src.plugin, src.isScaffold = plugin, true
	g.sources[id] = src
	return nil
}

// api describes the generated code as lay places it.
func (g *generation) api(lay *layout.Layout) *API {
	out := &API{Package: g.cfg.Package, UserContext: g.cfg.UserContext}
	for _, d := range g.m.Decls {
		out.Types = append(out.Types, typeRef(gomodel.DeclRef{Decl: d}, lay))
	}

	routed := make(map[string]bool)
	if g.srv != nil {
		for _, r := range g.srv.Routes() {
			routed[r.Operation] = true
		}
	}
	for _, op := range g.m.Operations {
		out.Operations = append(out.Operations, g.operation(op, lay, routed[op.Name]))
	}
	return out
}

func (g *generation) operation(op *gomodel.Operation, lay *layout.Layout, isRouted bool) Operation {
	o := Operation{
		ID:         op.Name,
		Method:     op.Spec.Method,
		Path:       op.Spec.Path,
		Summary:    op.Spec.Summary,
		Tags:       op.Spec.Tags,
		HasOptions: len(op.Params)+len(op.Bodies) > 0,
		IsRouted:   isRouted,
		Success:    success(op, lay),
	}
	if g.srv != nil {
		f := lay.FileOf(server.PartService)
		o.RequestOptions = TypeRef{Name: g.namer.ServiceRequestOptions(op.Name), Package: f.Package, ImportPath: f.ImportPath}
		o.ResponseData = TypeRef{Name: g.namer.ResponseData(op.Name), Package: f.Package, ImportPath: f.ImportPath}
	}
	if g.cl != nil && !op.Spec.IsWebhook {
		f := lay.FileOf(client.PartOptions)
		o.ClientRequestOptions = TypeRef{Name: g.namer.ClientRequestOptions(op.Name), Package: f.Package, ImportPath: f.ImportPath}
		if f = lay.FileOf(client.PartResponses); f != nil {
			o.ClientResponse = TypeRef{Name: g.namer.ClientResponse(op.Name), Package: f.Package, ImportPath: f.ImportPath}
		}
	}
	return o
}

// part renders one part of the file of s: a plugin's from its source, else a built-in one from
// its view.
func (g *generation) part(id layout.PartID, s *gocode.Scope) ([]byte, error) {
	src, ok := g.sources[id]
	if !ok {
		return g.engine.RenderPart(id, g.view(id, s))
	}

	data := src.data
	if src.isScaffold {
		data = g.view(id, s)
	}
	for _, imp := range src.imports {
		s.Import(gomodel.Import{Path: imp.Path, Alias: imp.Alias})
	}
	funcs := make(template.FuncMap, len(src.funcs)+2)
	maps.Copy(funcs, src.funcs)
	funcs["expr"] = func(t TypeRef) string {
		return s.Qualified(t.Name, gomodel.Import{Path: t.ImportPath, Alias: t.Package})
	}
	funcs["import"] = func(path string) string { return s.Import(gomodel.Import{Path: path}) }

	out, err := render.RenderSource(render.Source{Name: string(id), Text: src.text, Funcs: funcs}, data)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrPlugin, src.plugin, err)
	}
	return out, nil
}

// success is the first 2xx response of op, as the handlers answer it.
func success(op *gomodel.Operation, lay *layout.Layout) *Success {
	r, ok := operation.Success(op)
	if !ok {
		return nil
	}

	s := &Success{Status: r.Status}
	if r.Body != nil {
		s.ContentType = r.Body.MediaType
		s.Body = typeRef(operation.BodyType(*r.Body), lay)
		s.IsRaw = r.Body.Type == nil
	}
	return s
}

// typeRef describes t: its text as its own package writes it, and the package of the named type
// inside it.
func typeRef(t gomodel.Type, lay *layout.Layout) TypeRef {
	ref := TypeRef{Name: gocode.Text(t)}
	switch leaf := gocode.Leaf(t).(type) {
	case gomodel.DeclRef:
		if f := lay.FileOf(layout.PartID(leaf.Decl.Part)); f != nil {
			ref.Package, ref.ImportPath = f.Package, f.ImportPath
		}
	case gomodel.Qualified:
		ref.Package, ref.ImportPath = gocode.ImportName(leaf.Import.Path, leaf.Import.Alias), leaf.Import.Path
	}
	return ref
}
