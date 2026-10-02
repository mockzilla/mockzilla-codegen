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
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
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

// render runs the template on data and checks that it wrote Go declarations. Its expr and import
// funcs write for the file of s; a func of the plugin with either name replaces it.
func (src source) render(id layout.PartID, data any, s *gocode.Scope) ([]byte, error) {
	for _, imp := range src.imports {
		s.Import(gomodel.Import{Path: imp.Path, Alias: imp.Alias})
	}

	funcs := make(template.FuncMap, len(src.funcs)+2)
	funcs["expr"] = func(t TypeRef) (string, error) {
		if err := t.check(); err != nil {
			return "", err
		}
		return s.Qualified(t.Name, gomodel.Import{Path: t.ImportPath, Alias: t.Package}), nil
	}
	funcs["import"] = func(path string) (string, error) {
		if err := (Import{Path: path}).check(); err != nil {
			return "", err
		}
		return s.Import(gomodel.Import{Path: path}), nil
	}
	maps.Copy(funcs, src.funcs)

	out, err := render.RenderSource(render.Source{Name: string(id), Text: src.text, Funcs: funcs}, data)
	if err == nil {
		err = gocode.CheckDecls(string(id), out)
	}
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrPlugin, src.plugin, err)
	}
	return out, nil
}

// pluginSet is the plugins of one run and what they reserve and give. idents come in the order
// the plugins are given; fields holds the request option fields by operation name, each list in
// that order too; sources holds the template of every part they add and of every scaffold they
// replace.
type pluginSet struct {
	list    []Plugin
	idents  []string
	fields  map[string][]server.ExtraField
	sources map[layout.PartID]source
}

func newPluginSet(list []Plugin) pluginSet {
	return pluginSet{list: list, fields: make(map[string][]server.ExtraField), sources: make(map[layout.PartID]source)}
}

// reserve checks the plugins' names and collects what they reserve.
func (ps *pluginSet) reserve() error {
	var names []string
	for _, p := range ps.list {
		name := p.Name()
		switch {
		case !segment.MatchString(name):
			return fmt.Errorf("%w %q: the name must match [a-z][a-z0-9]*", ErrPlugin, name)
		case slices.Contains(names, name):
			return fmt.Errorf("%w %s: the name is used twice", ErrPlugin, name)
		}
		names = append(names, name)

		res := p.Reserve()
		for _, ident := range res.Idents {
			if !token.IsIdentifier(ident) {
				return fmt.Errorf("%w %s: reserved name %q is no identifier", ErrPlugin, name, ident)
			}
		}
		ps.idents = append(ps.idents, res.Idents...)
	}
	return nil
}

// contribute shows every plugin the API and keeps what each gives. It returns the parts the
// plugins add. show is called once per plugin, so that none sees what another did to its API.
func (ps *pluginSet) contribute(show func() *API) ([]layout.Part, error) {
	var parts []layout.Part
	for _, p := range ps.list {
		api := show()
		// Listed before the plugin gets the API, which it is free to change.
		ops := make(map[string]bool, len(api.Operations))
		for _, op := range api.Operations {
			ops[op.ID] = true
		}

		c, err := p.Contribute(api)
		if err != nil {
			return nil, fmt.Errorf("%w %s: %w", ErrPlugin, p.Name(), err)
		}
		if c == nil {
			continue
		}

		// A copy, so the funcs that are checked here are the ones the templates get.
		funcs := maps.Clone(c.Funcs)
		if err = render.CheckFuncs(funcs); err != nil {
			return nil, fmt.Errorf("%w %s: %w", ErrPlugin, p.Name(), err)
		}

		for _, src := range c.Parts {
			id, addErr := ps.addPart(p.Name(), src, funcs)
			if addErr != nil {
				return nil, addErr
			}
			parts = append(parts, layout.Part{ID: id})
		}
		if err = ps.addFields(p.Name(), c.RequestOptionFields, ops); err != nil {
			return nil, err
		}
		for _, kind := range slices.Sorted(maps.Keys(c.Scaffolds)) {
			if err = ps.replaceScaffold(p.Name(), kind, source{text: c.Scaffolds[kind], funcs: funcs}); err != nil {
				return nil, err
			}
		}
	}
	return parts, nil
}

func (ps *pluginSet) addPart(plugin string, src PartSource, funcs template.FuncMap) (layout.PartID, error) {
	id := layout.PartID("plugin." + plugin + "." + src.Name)
	switch _, taken := ps.sources[id]; {
	case !segment.MatchString(src.Name):
		return "", fmt.Errorf("%w %s: the part name %q must match [a-z][a-z0-9]*", ErrPlugin, plugin, src.Name)
	case taken:
		return "", fmt.Errorf("%w %s: the part %s is contributed twice", ErrPlugin, plugin, src.Name)
	}

	// A copy, so the imports that are checked here are the ones the file gets.
	imports := slices.Clone(src.Imports)
	for _, imp := range imports {
		if err := imp.check(); err != nil {
			return "", fmt.Errorf("%w %s: the part %s: %w", ErrPlugin, plugin, src.Name, err)
		}
	}

	ps.sources[id] = source{plugin: plugin, text: src.Template, funcs: funcs, imports: imports, data: src.Data}
	return id, nil
}

// addFields keeps the request option fields a plugin gives. A key has to be one of ops, the
// operation IDs of the API.
func (ps *pluginSet) addFields(plugin string, fields map[string][]FieldSpec, ops map[string]bool) error {
	for _, op := range slices.Sorted(maps.Keys(fields)) {
		if !ops[op] {
			return fmt.Errorf("%w %s: request option fields for %q, which is no operation ID of the API", ErrPlugin, plugin, op)
		}
		for _, f := range fields[op] {
			if err := ps.addField(plugin, op, f); err != nil {
				return err
			}
		}
	}
	return nil
}

func (ps *pluginSet) addField(plugin, op string, f FieldSpec) error {
	switch {
	case !token.IsIdentifier(f.Name) || !token.IsExported(f.Name):
		return fmt.Errorf("%w %s: request option field %q of %s is no exported identifier", ErrPlugin, plugin, f.Name, op)
	case f.Type.Name == "":
		return fmt.Errorf("%w %s: request option field %s of %s has no type", ErrPlugin, plugin, f.Name, op)
	case f.Type.ImportPath == "" && f.Type.Package != "":
		return fmt.Errorf("%w %s: request option field %s of %s has a type of package %s without an import path", ErrPlugin, plugin, f.Name, op, f.Type.Package)
	case !gocode.IsFieldType(f.Type.Name):
		return fmt.Errorf("%w %s: request option field %s of %s: type %q is no Go type", ErrPlugin, plugin, f.Name, op, f.Type.Name)
	case server.ReservedField(f.Name):
		return fmt.Errorf("%w %s: request option field %s of %s is one the request options declare themselves", ErrPlugin, plugin, f.Name, op)
	case slices.ContainsFunc(ps.fields[op], func(x server.ExtraField) bool { return x.Name == f.Name }):
		return fmt.Errorf("%w %s: request option field %s of %s is added twice", ErrPlugin, plugin, f.Name, op)
	}
	if err := f.Type.check(); err != nil {
		return fmt.Errorf("%w %s: request option field %s of %s: %w", ErrPlugin, plugin, f.Name, op, err)
	}

	ps.fields[op] = append(ps.fields[op], server.ExtraField{
		Name:   f.Name,
		Type:   f.Type.Name,
		Doc:    f.Doc,
		Import: gomodel.Import{Path: f.Type.ImportPath, Alias: f.Type.Package},
	})
	return nil
}

func (ps *pluginSet) replaceScaffold(plugin string, kind ScaffoldKind, src source) error {
	id, ok := scaffoldParts[kind]
	if !ok {
		return fmt.Errorf("%w %s: %d is no scaffold kind", ErrPlugin, plugin, kind)
	}
	if other, taken := ps.sources[id]; taken {
		return fmt.Errorf("%w %s: the %s scaffold is already replaced by %s", ErrPlugin, plugin, kind, other.plugin)
	}

	src.plugin, src.isScaffold = plugin, true
	ps.sources[id] = src
	return nil
}

// describe is what a plugin sees of the code g generates, as lay places it. The value shares
// nothing with the model, the config or the value of another call.
func describe(g *generation, lay *layout.Layout) *API {
	out := &API{Package: lay.Package, UserContext: copyMap(g.cfg.UserContext)}
	for _, d := range g.m.Decls {
		out.Types = append(out.Types, typeRef(gomodel.DeclRef{Decl: d}, lay))
	}

	routed := make(map[string]bool)
	if g.srv != nil {
		if f := lay.FileOf(server.PartService); f != nil {
			out.Service = TypeRef{Name: g.srv.Interface(), Package: f.Package, ImportPath: f.ImportPath}
		}

		for _, r := range g.srv.Routes() {
			routed[r.Operation] = true
		}
	}
	for _, op := range g.m.Operations {
		out.Operations = append(out.Operations, describeOperation(g.namer, op, lay, routed[op.Name]))
	}
	return out
}

// describeOperation leaves empty the types of a part lay does not hold: the config asks for no
// server, no client or no envelopes.
func describeOperation(namer *naming.Namer, op *gomodel.Operation, lay *layout.Layout, isRouted bool) Operation {
	o := Operation{
		ID:         op.Name,
		Method:     op.Spec.Method,
		Path:       op.Spec.Path,
		Summary:    op.Spec.Summary,
		Tags:       slices.Clone(op.Spec.Tags),
		HasOptions: len(op.Params)+len(op.Bodies) > 0,
		IsRouted:   isRouted,
		Success:    success(op, lay),
	}
	if f := lay.FileOf(server.PartService); f != nil {
		o.RequestOptions = TypeRef{Name: namer.ServiceRequestOptions(op.Name), Package: f.Package, ImportPath: f.ImportPath}
		o.ResponseData = TypeRef{Name: namer.ResponseData(op.Name), Package: f.Package, ImportPath: f.ImportPath}
	}
	if op.Spec.IsWebhook {
		return o
	}

	if f := lay.FileOf(client.PartOptions); f != nil {
		o.ClientRequestOptions = TypeRef{Name: namer.ClientRequestOptions(op.Name), Package: f.Package, ImportPath: f.ImportPath}
	}
	if f := lay.FileOf(client.PartResponses); f != nil {
		o.ClientResponse = TypeRef{Name: namer.ClientResponse(op.Name), Package: f.Package, ImportPath: f.ImportPath}
	}
	return o
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

// copyMap copies m with the maps and lists in it, of the kinds YAML decodes to. A value of
// another kind, which only Go code can put there, stays shared.
func copyMap[K comparable](m map[K]any) map[K]any {
	out := make(map[K]any, len(m))
	for k, v := range m {
		out[k] = copyValue(v)
	}
	return out
}

func copyValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return copyMap(x)
	case map[any]any:
		return copyMap(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = copyValue(e)
		}
		return out
	}
	return v
}
