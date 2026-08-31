// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"fmt"
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// declRule says when a schema met at the start of a walk gets a declaration of its own.
type declRule int

const (
	// ruleIfNeeded names only enums, structs and unions.
	ruleIfNeeded declRule = iota
	// ruleUnlessRef names every shape but a plain $ref, which uses the referenced type.
	ruleUnlessRef
	ruleAlways
)

var paramOrder = []string{spec.InPath, spec.InQuery, spec.InQueryString, spec.InHeader, spec.InCookie}

// pending is a declaration found by the walk. Its name is base's final name plus name, or name
// alone without a base, so an inline type follows a renamed parent.
type pending struct {
	decl     *Decl
	schema   *spec.Schema
	params   []*spec.Parameter
	shape    shape
	base     *pending
	name     string
	fallback string
	rank     naming.Rank
	order    int
	depth    int
}

// place is where a schema sits, for naming: the nearest declaration above and the name added
// since. Fallback and rank apply to the schema at this place only, never to its children.
type place struct {
	base     *pending
	name     string
	fallback string
	rank     naming.Rank
	part     string
}

func (p place) child(suffix string) place {
	return place{base: p.base, name: p.name + suffix, rank: naming.RankInline, part: p.part}
}

// childSchema is a schema under another one with the name part it adds.
type childSchema struct {
	schema *spec.Schema
	suffix string
}

// collector walks the spec in order, components first, and lists every declaration to make.
// It walks exactly the schemas whose types the builder computes later.
type collector struct {
	doc         *spec.Document
	isServer    bool
	namer       *naming.Namer
	flat        *flattener
	unions      *unionReader
	ext         *extReader
	diags       *diag.Collector
	pending     []*pending
	headers     map[*spec.Response]*Decl
	bySchema    map[*spec.Schema]*pending
	visited     map[*spec.Schema]bool
	onStack     map[*spec.Schema]place
	isComponent map[*spec.Schema]bool
}

func newCollector(doc *spec.Document, r readers, diags *diag.Collector) *collector {
	c := &collector{
		doc:         doc,
		isServer:    r.isServer,
		namer:       r.unions.namer,
		flat:        r.flat,
		unions:      r.unions,
		ext:         r.ext,
		diags:       diags,
		headers:     map[*spec.Response]*Decl{},
		bySchema:    map[*spec.Schema]*pending{},
		visited:     map[*spec.Schema]bool{},
		onStack:     map[*spec.Schema]place{},
		isComponent: map[*spec.Schema]bool{},
	}
	for _, s := range doc.Components.Schemas {
		c.isComponent[s.Value] = true
	}
	return c
}

// run walks components, then each operation: its parameters, bodies and responses.
func (c *collector) run(ops []*Operation) {
	n := c.namer
	for _, s := range c.doc.Components.Schemas {
		name := n.Exported(s.Name)
		c.walk(s.Value, place{name: name, fallback: name + "Schema", rank: naming.RankComponentSchema, part: PartTypes}, ruleAlways)
	}
	for _, p := range c.doc.Components.Parameters {
		name := n.Exported(p.Name)
		c.walk(paramSchema(p.Value), place{name: name, fallback: name + "Parameter", rank: naming.RankComponent, part: PartParams}, ruleIfNeeded)
	}
	for _, b := range c.doc.Components.RequestBodies {
		c.contents(b.Name, b.Value.Contents, "RequestBody", PartBodies)
	}
	for _, r := range c.doc.Components.Responses {
		c.contents(r.Name, r.Value.Contents, "Response", PartResponses)
	}

	for _, op := range ops {
		c.params(op)
		if b := op.Spec.Body; b != nil {
			isMultiple := countInline(b.Contents) > 1
			for _, mt := range b.Contents {
				at := place{name: n.RequestBody(op.Name, mt.Name, isMultiple), rank: naming.RankOperation, part: PartBodies}
				c.walk(mt.Schema, at, ruleUnlessRef)
			}
		}
		for _, r := range op.Spec.Responses {
			isMultiple := countInline(r.Contents) > 1
			for _, mt := range r.Contents {
				at := place{name: n.Response(op.Name, r.Status, mt.Name, isMultiple), rank: naming.RankOperation, part: PartResponses}
				c.walk(mt.Schema, at, ruleUnlessRef)
			}
			if c.isServer && len(r.Headers) > 0 {
				c.responseHeaders(op, r)
			}
		}
	}
}

// responseHeaders adds the struct of the typed headers of a response and walks their schemas.
func (c *collector) responseHeaders(op *Operation, r *spec.Response) {
	list := make([]*spec.Parameter, len(r.Headers))
	for i, h := range r.Headers {
		list[i] = &spec.Parameter{
			Name:        h.Name,
			In:          spec.InHeader,
			Description: h.Description,
			Required:    h.Required,
			Deprecated:  h.Deprecated,
			Style:       h.Style,
			Explode:     h.Explode,
			Schema:      h.Schema,
			Contents:    h.Contents,
			Extensions:  h.Extensions,
			Origin:      h.Origin,
		}
	}

	d := &Decl{ID: r.Origin.Pointer + "/headers", Kind: KindStruct, Part: PartResponses, Origin: origin(r.Origin)}
	p := c.push(&pending{decl: d, params: list, name: c.namer.ResponseHeaders(op.Name, r.Status), rank: naming.RankOperation})
	c.headers[r] = d

	at := place{base: p, rank: naming.RankInline, part: PartResponses}
	for _, h := range list {
		c.walk(paramSchema(h), at.child(c.namer.InlineProperty("", h.Name)), ruleIfNeeded)
	}
}

// contents walks a component request body or response, named after the component.
func (c *collector) contents(name string, contents []*spec.MediaType, kind, part string) {
	base := c.namer.Exported(name)
	isMultiple := countInline(contents) > 1
	for _, mt := range contents {
		want := base
		if isMultiple {
			want += c.namer.MediaTag(mt.Name)
		}
		c.walk(mt.Schema, place{name: want, fallback: want + kind, rank: naming.RankComponent, part: part}, ruleUnlessRef)
	}
}

// params adds one struct per parameter location and walks the parameter schemas under it.
func (c *collector) params(op *Operation) {
	for _, in := range paramOrder {
		var list []*spec.Parameter
		seen := map[string]bool{}
		for _, p := range op.Spec.Params {
			switch {
			case p.In != in:
				continue
			case seen[p.Name]:
				c.diags.Append(diag.Diagnostic{
					Severity: diag.Warning,
					Code:     diag.CodeDuplicateParam,
					Pointer:  p.Origin.Pointer,
					Origin:   origin(p.Origin),
					Message:  fmt.Sprintf("%s parameter %q is listed again; only the first one is used", in, p.Name),
				})
				continue
			}
			seen[p.Name] = true
			list = append(list, p)
		}
		if len(list) == 0 {
			continue
		}

		d := &Decl{ID: op.Spec.Origin.Pointer + "/parameters/" + in, Kind: KindStruct, Part: PartParams, Origin: origin(op.Spec.Origin)}
		p := c.push(&pending{decl: d, params: list, name: c.namer.Params(op.Name, in), rank: naming.RankOperation})
		op.Params = append(op.Params, ParamGroup{In: in, Decl: d})

		at := place{base: p, rank: naming.RankInline, part: PartParams}
		for _, param := range list {
			c.walk(paramSchema(param), at.child(c.namer.InlineProperty("", param.Name)), ruleIfNeeded)
		}
	}
}

// walk visits a schema once and names the children its type is built from. In a merged schema,
// children that came from a type reached through a $ref are left to that type. A schema met
// again while it is still being walked closes a loop of refs: it gets a declaration, so its type
// stays finite.
func (c *collector) walk(s *spec.Schema, at place, rule declRule) {
	if s == nil {
		return
	}
	if c.visited[s] {
		c.closeLoop(s)
		return
	}
	c.visited[s] = true
	if c.ext.of(s.Extensions, s.Origin).GoType != nil {
		if rule == ruleAlways {
			c.add(s, at, shapeAny)
		}
		return
	}
	c.onStack[s] = at
	defer delete(c.onStack, s)

	f := c.flat.flatten(s)
	sh := classify(f)
	if c.needsDecl(s, sh, rule) {
		at = c.add(s, at, sh)
	}
	c.checkEnum(f, sh)

	refs, foreign := []*spec.Ref{s.Ref}, map[*spec.Schema]bool(nil)
	if m := c.flat.merged(s); m != nil {
		refs, foreign = m.refs, m.foreign
	}
	for _, r := range refs {
		c.follow(r, at)
	}
	children := c.children(f)
	if sh == shapeUnion {
		children = c.unionChildren(f)
	}
	for _, ch := range children {
		if !foreign[ch.schema] {
			c.walk(ch.schema, at.child(ch.suffix), ruleIfNeeded)
		}
	}
}

func (c *collector) needsDecl(s *spec.Schema, sh shape, rule declRule) bool {
	switch {
	case rule == ruleAlways:
		return true
	case refOf(s) != nil:
		return false
	case rule == ruleUnlessRef:
		return true
	}
	return sh == shapeEnum || sh == shapeStruct || sh == shapeUnion
}

// add registers a declaration for s and returns the place its children are named from.
func (c *collector) add(s *spec.Schema, at place, sh shape) place {
	d := &Decl{ID: s.Origin.Pointer, Part: partOf(sh, at.part), Origin: origin(s.Origin)}
	p := &pending{decl: d, schema: s, shape: sh, base: at.base, name: at.name, fallback: at.fallback, rank: at.rank}
	if name := c.goName(s, at); name != "" {
		p.base, p.name, p.fallback, p.rank = nil, name, "", naming.RankGoName
	}
	c.bySchema[s] = c.push(p)
	return place{base: p, rank: naming.RankInline, part: at.part}
}

// goName is the type name x-go-type-name gives s, or x-go-name where s is named on its own, as a
// component or a body is.
func (c *collector) goName(s *spec.Schema, at place) string {
	set := c.ext.of(s.Extensions, s.Origin)
	name := set.TypeName
	if name == "" && at.base == nil {
		name = set.Name
	}
	return c.ext.goName(set, name)
}

func (c *collector) push(p *pending) *pending {
	p.order = len(c.pending)
	if p.base != nil {
		p.depth = p.base.depth + 1
	}
	c.pending = append(c.pending, p)
	return p
}

func (c *collector) closeLoop(s *spec.Schema) {
	at, ok := c.onStack[s]
	if !ok || c.bySchema[s] != nil {
		return
	}
	c.add(s, at, classify(c.flat.flatten(s)))
}

// follow walks the target of a ref to anything but a component schema, which is walked on its own.
func (c *collector) follow(r *spec.Ref, at place) {
	if r == nil || c.isComponent[r.Target] {
		return
	}
	c.walk(r.Target, at, ruleIfNeeded)
}

func (c *collector) children(s *spec.Schema) []childSchema {
	out := make([]childSchema, 0, len(s.Properties)+2)
	for _, p := range s.Properties {
		out = append(out, childSchema{schema: p.Schema, suffix: c.namer.InlineProperty("", p.Name)})
	}
	if s.Items != nil {
		out = append(out, childSchema{schema: s.Items, suffix: c.namer.ArrayItem("")})
	}
	if s.AdditionalProperties.Mode == spec.AdditionalSchema {
		out = append(out, childSchema{schema: s.AdditionalProperties.Schema, suffix: c.namer.MapValue("")})
	}
	return out
}

// unionChildren are the members of a union, then the properties they share.
func (c *collector) unionChildren(f *spec.Schema) []childSchema {
	u := c.unions.read(f)
	out := make([]childSchema, 0, len(u.members)+len(f.Properties))
	for _, m := range u.members {
		out = append(out, childSchema{schema: m.schema, suffix: m.suffix})
	}
	if u.isTypeList {
		return out
	}
	for _, p := range f.Properties {
		out = append(out, childSchema{schema: p.Schema, suffix: c.namer.InlineProperty("", p.Name)})
	}
	return out
}

// checkEnum reports an enum that cannot become constants, such as one on an object.
func (c *collector) checkEnum(f *spec.Schema, sh shape) {
	isNotNull := func(v spec.Value) bool { return v.Kind != spec.KindNull }
	if sh == shapeEnum || sh == shapeUnion || !slices.ContainsFunc(f.Enum, isNotNull) {
		return
	}

	msg := "enum values have no common constant type; the enum is left out"
	if t := f.Types &^ spec.TypeNull; t != 0 {
		msg = "enum values do not fit type " + typeSetText(t) + "; the enum is left out"
	}
	c.diags.Append(diag.Diagnostic{
		Severity: diag.Warning,
		Code:     diag.CodeEnumIgnored,
		Pointer:  f.Origin.Pointer,
		Origin:   origin(f.Origin),
		Message:  msg,
	})
}

// partOf puts enums and unions in parts of their own; other types stay with where they come from.
func partOf(sh shape, from string) string {
	switch sh {
	case shapeEnum:
		return PartEnums
	case shapeUnion:
		return PartUnions
	default:
		return from
	}
}

// paramSchema is a parameter's schema, or the schema of its one media type.
func paramSchema(p *spec.Parameter) *spec.Schema {
	if p.Schema == nil && len(p.Contents) > 0 {
		return p.Contents[0].Schema
	}
	return p.Schema
}

// countInline counts media types whose schema is more than a plain $ref.
func countInline(contents []*spec.MediaType) int {
	n := 0
	for _, mt := range contents {
		if mt.Schema != nil && refOf(mt.Schema) == nil {
			n++
		}
	}
	return n
}
