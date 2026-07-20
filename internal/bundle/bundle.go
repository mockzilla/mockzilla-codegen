// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package bundle turns a spec split over files and URLs into one document.
package bundle

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
)

const refKey = "$ref"

// Loader reads a file path or URL.
type Loader func(ctx context.Context, location string) ([]byte, error)

// Input is the root spec. Its refs resolve against BaseDir when set, else next to Location.
type Input struct {
	Doc      *oasdoc.Doc
	Location string
	BaseDir  string
	Load     Loader
}

// source is one document: the root or a file or URL a ref points into.
type source struct {
	loc  string
	base string
	doc  *oasdoc.Doc
}

// target is a node in some document; frag is a JSON pointer, empty for the whole document.
type target struct {
	loc  string
	frag string
}

// entry is one component of the root: components/<section>/<name>.
type entry struct {
	section string
	name    string
	value   *yaml.Node
}

// resolved is where a ref leads after following refs that hold nothing else. name comes from
// the first ref of the chain; isRoot means it ends in the root document.
type resolved struct {
	target
	src    *source
	node   *yaml.Node
	name   string
	isRoot bool
}

type bundler struct {
	ctx       context.Context
	load      Loader
	root      *source
	sources   map[string]*source
	homes     map[target]string
	names     map[string]map[string]bool
	pending   map[*yaml.Node]*source
	lifted    map[*yaml.Node]bool
	inlining  []target
	diags     diag.Collector
	err       error
	isChanged bool
}

// Run lifts what the root's refs point at in other files into its components, in place, and
// inlines path items. It reports whether the document changed.
func Run(ctx context.Context, in Input) (bool, []diag.Diagnostic, error) {
	if !hasExternalRef(in.Doc) {
		return false, nil, nil
	}

	b := newBundler(ctx, in)
	b.home()
	b.walk(in.Doc.Root(), "", oasdoc.KindDocument, b.root)
	if b.err != nil {
		return false, nil, b.err
	}
	b.warnLeftovers()
	return b.isChanged, b.diags.List(), nil
}

func newBundler(ctx context.Context, in Input) *bundler {
	loc, base := in.Location, in.BaseDir
	if loc != "" && !IsURL(loc) {
		loc = filepath.Clean(loc)
	}
	switch {
	case base != "":
	case loc != "":
		base = baseOf(loc)
	default:
		base = "."
	}

	root := &source{loc: loc, base: base, doc: in.Doc}
	return &bundler{
		ctx:     ctx,
		load:    in.Load,
		root:    root,
		sources: map[string]*source{loc: root},
		homes:   map[target]string{},
		names:   map[string]map[string]bool{},
		pending: map[*yaml.Node]*source{},
		lifted:  map[*yaml.Node]bool{},
	}
}

// home reserves the root's component names, then gives each root component that is only a ref
// to another file the content it points at, so that content keeps the root's name.
func (b *bundler) home() {
	comps := oasdoc.Child(b.root.doc.Root(), "components")
	entries := sectionEntries(comps)
	for _, e := range entries {
		b.taken(e.section)[e.name] = true
	}

	for _, e := range entries {
		ref := pureRef(e.value)
		if ref == "" {
			continue
		}
		r, err := b.follow(b.root, ref)
		if err != nil || r.isRoot {
			continue
		}
		if _, ok := b.homes[r.target]; ok {
			continue
		}

		content := oasdoc.Clone(r.node)
		oasdoc.SetChild(oasdoc.Child(comps, e.section), e.name, content)
		b.root.doc.SetFile(content, r.loc)
		b.pending[content] = r.src
		b.homes[r.target] = componentRef(e.section, e.name)
		b.isChanged = true
	}
}

// walk visits n with src as the document its refs are relative to. Content grafted from another
// document is walked once, with its own document.
func (b *bundler) walk(n *yaml.Node, ptr string, k oasdoc.Kind, src *source) {
	oasdoc.Visit(n, ptr, k, func(at string, m *yaml.Node, mk oasdoc.Kind) bool {
		if b.err != nil {
			return false
		}
		if m != n {
			if b.lifted[m] {
				return false
			}
			if s, ok := b.pending[m]; ok {
				delete(b.pending, m)
				b.walk(m, at, mk, s)
				return false
			}
		}
		return b.visit(at, m, mk, src)
	})
}

func (b *bundler) visit(ptr string, n *yaml.Node, k oasdoc.Kind, src *source) bool {
	if k == oasdoc.KindSchema {
		b.mapping(n, src)
	}

	v := oasdoc.Child(n, refKey)
	if v == nil || v.Kind != yaml.ScalarNode || (src == b.root && strings.HasPrefix(v.Value, "#")) {
		return true
	}

	r, err := b.follow(src, v.Value)
	if err != nil {
		b.fail(src, v, err)
		return false
	}
	if r.isRoot {
		b.rewrite(v, "#"+r.frag)
		return true
	}
	if k != oasdoc.KindPathItem && k.Section() != "" {
		b.rewrite(v, b.lift(r, k))
		return true
	}

	switch {
	case slices.Contains(b.inlining, r.target):
		b.fail(src, v, fmt.Errorf("%w: %s#%s inlines itself", ErrCycle, r.loc, r.frag))
	case r.node.Kind != yaml.MappingNode:
		b.fail(src, v, fmt.Errorf("%w: %s#%s", ErrInline, r.loc, r.frag))
	default:
		b.inline(ptr, n, k, r)
	}
	return false
}

// mapping lifts discriminator targets in other files. A bare name in another document means a
// schema in that document's components.
func (b *bundler) mapping(n *yaml.Node, src *source) {
	m := oasdoc.Child(oasdoc.Child(n, "discriminator"), "mapping")
	if m == nil {
		return
	}

	for i := 1; i < len(m.Content) && b.err == nil; i += 2 {
		v := m.Content[i]
		ref := v.Value
		switch {
		case strings.ContainsAny(ref, "#/") || isFileName(ref):
		case src != b.root && src.doc.Get("/components/schemas/"+oasdoc.Escape(ref)) != nil:
			ref = "#/components/schemas/" + oasdoc.Escape(ref)
		default:
			continue
		}
		if src == b.root && strings.HasPrefix(ref, "#") {
			continue
		}

		r, err := b.follow(src, ref)
		switch {
		case err != nil:
			b.fail(src, v, err)
		case r.isRoot:
			b.rewrite(v, "#"+r.frag)
		default:
			b.rewrite(v, b.lift(r, oasdoc.KindSchema))
		}
	}
}

// follow resolves ref, then keeps going while the target holds only another ref.
func (b *bundler) follow(src *source, ref string) (resolved, error) {
	var r resolved
	seen := map[target]bool{}
	for {
		t, err := b.resolve(src, ref)
		if err != nil {
			return r, err
		}
		if r.name == "" {
			r.name = nameOf(t)
		}
		if seen[t] {
			return r, fmt.Errorf("%w: %s", ErrCycle, ref)
		}
		seen[t] = true

		if t.loc == b.root.loc {
			r.target, r.isRoot = t, true
			return r, nil
		}
		s, err := b.source(t.loc)
		if err != nil {
			return r, err
		}
		n := s.doc.Get("#" + t.frag)
		if n == nil {
			return r, fmt.Errorf("%w: %s#%s", ErrTarget, t.loc, t.frag)
		}

		r.target, r.src, r.node = t, s, n
		if ref = pureRef(n); ref == "" {
			return r, nil
		}
		src = s
	}
}

func (b *bundler) resolve(src *source, ref string) (target, error) {
	file, frag, _ := strings.Cut(ref, "#")
	if frag != "" && !strings.HasPrefix(frag, "/") {
		return target{}, fmt.Errorf("%w: %s", ErrFragment, ref)
	}
	if file == "" {
		return target{loc: src.loc, frag: frag}, nil
	}

	loc, err := join(src.base, file)
	if err != nil {
		return target{}, fmt.Errorf("%w: %s: %w", ErrLoad, ref, err)
	}
	return target{loc: loc, frag: frag}, nil
}

func (b *bundler) source(loc string) (*source, error) {
	if s, ok := b.sources[loc]; ok {
		return s, nil
	}

	data, err := b.load(b.ctx, loc)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoad, err)
	}
	doc, err := oasdoc.Parse(data, loc)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoad, err)
	}

	s := &source{loc: loc, base: baseOf(loc), doc: doc}
	b.sources[loc] = s
	return s, nil
}

// lift copies the target into components once and returns the ref to it.
func (b *bundler) lift(r resolved, k oasdoc.Kind) string {
	if home, ok := b.homes[r.target]; ok {
		return home
	}

	section := k.Section()
	name := b.claim(section, r)
	root := b.root.doc.Root()
	comps := oasdoc.Child(root, "components")
	if comps == nil {
		comps = oasdoc.NewMapping()
		oasdoc.SetChild(root, "components", comps)
	}
	sec := oasdoc.Child(comps, section)
	if sec == nil {
		sec = oasdoc.NewMapping()
		oasdoc.SetChild(comps, section, sec)
	}

	content := oasdoc.Clone(r.node)
	oasdoc.SetChild(sec, name, content)
	b.root.doc.SetFile(content, r.loc)
	home := componentRef(section, name)
	b.homes[r.target] = home
	b.lifted[content] = true
	b.isChanged = true
	b.walk(content, home[1:], k, r.src)
	return home
}

// claim picks a free component name: the plain name, then with the file name, then with each
// folder from the nearest, then numbered.
func (b *bundler) claim(section string, r resolved) string {
	taken := b.taken(section)
	candidates := []string{r.name}
	if s := title(stem(r.loc)); s != "" && !strings.EqualFold(s, r.name) {
		candidates = append(candidates, r.name+s)
	}
	for _, f := range folders(r.loc, b.root.base) {
		candidates = append(candidates, r.name+title(f))
	}

	name := ""
	for _, c := range candidates {
		if !taken[c] {
			name = c
			break
		}
	}
	for i := 2; name == ""; i++ {
		if c := r.name + strconv.Itoa(i); !taken[c] {
			name = c
		}
	}

	taken[name] = true
	if name != r.name {
		b.diags.Append(diag.Diagnostic{
			Severity: diag.Info,
			Code:     diag.CodeBundleRename,
			Pointer:  componentRef(section, name)[1:],
			Origin:   diag.Origin{File: r.loc, Line: r.node.Line, Col: r.node.Column},
			Message:  fmt.Sprintf("%s from %s is named %s: %s is taken", r.name, r.loc, name, r.name),
		})
	}
	return name
}

func (b *bundler) taken(section string) map[string]bool {
	if b.names[section] == nil {
		b.names[section] = map[string]bool{}
	}
	return b.names[section]
}

// inline replaces the ref object n with a copy of its target; keys next to the $ref win.
func (b *bundler) inline(ptr string, n *yaml.Node, k oasdoc.Kind, r resolved) {
	content := oasdoc.Clone(r.node)
	for i := 0; i+1 < len(n.Content); i += 2 {
		if key := n.Content[i].Value; key != refKey {
			oasdoc.SetChild(content, key, n.Content[i+1])
		}
	}
	n.Content, n.Style, n.Line, n.Column = content.Content, content.Style, content.Line, content.Column
	b.root.doc.SetFile(n, r.loc)
	b.isChanged = true

	b.inlining = append(b.inlining, r.target)
	b.walk(n, ptr, k, r.src)
	b.inlining = b.inlining[:len(b.inlining)-1]
}

func (b *bundler) rewrite(v *yaml.Node, ref string) {
	if v.Value != ref {
		v.Value = ref
		b.isChanged = true
	}
}

func (b *bundler) fail(src *source, n *yaml.Node, err error) {
	if b.err == nil {
		b.err = fmt.Errorf("%s:%d:%d: %w", src.loc, n.Line, n.Column, err)
	}
}

// warnLeftovers reports refs to other files in places the grammar does not follow.
func (b *bundler) warnLeftovers() {
	var left []oasdoc.Ref
	for _, r := range b.root.doc.Refs() {
		if !strings.HasPrefix(r.Value, "#") {
			left = append(left, r)
		}
	}
	if len(left) == 0 {
		return
	}

	positions := b.root.doc.Positions()
	for _, r := range left {
		b.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeUnbundledRef,
			Pointer:  r.Owner,
			Origin:   positions[r.Owner],
			Message:  "$ref " + r.Value + " is not in a place that is bundled, so it is left as is",
		})
	}
}
