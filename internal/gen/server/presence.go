// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of the bodyPresence table, which the adapter checks and fills request bodies from.

package server

import (
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

// PresenceView is the bodyPresence table. IsChecked adds the checks of validation.request.
type PresenceView struct {
	Runtime   string
	IsChecked bool
	Objects   []ObjectView
}

// ObjectView is one object of the table, with its name quoted.
type ObjectView struct {
	Name     string
	Props    []*PropView
	Extra    *PropView
	IsClosed bool
}

// PropView is a runtime.Prop. Key, Default and Object are quoted.
type PropView struct {
	Runtime    string
	Key        string
	IsRequired bool
	IsNullable bool
	Default    string
	Object     string
	Items      *PropView
	Values     *PropView
}

// presenceTable collects the objects the handlers check bodies against, and those with defaults.
type presenceTable struct {
	runtime    string
	isChecked  bool
	hasRoots   bool
	hasDefault map[*gomodel.Decl]bool
	isUsed     map[*gomodel.Decl]bool
	used       []*gomodel.Decl
}

func newPresenceTable(ops []*gomodel.Operation, isChecked bool, runtime string) *presenceTable {
	t := &presenceTable{runtime: runtime, isChecked: isChecked, hasDefault: map[*gomodel.Decl]bool{}, isUsed: map[*gomodel.Decl]bool{}}
	if isChecked {
		return t
	}

	var decls []*gomodel.Decl
	seen := map[*gomodel.Decl]bool{}
	for _, op := range ops {
		for _, c := range op.Bodies {
			decls = reach(c.Body, seen, decls)
		}
	}
	for isGrown := true; isGrown; {
		isGrown = false
		for _, d := range decls {
			if !t.hasDefault[d] && t.declLeads(d) {
				t.hasDefault[d], isGrown = true, true
			}
		}
	}
	return t
}

// root is the Prop a body of the given kind is checked against, nil when there is nothing to do.
func (t *presenceTable) root(c gomodel.Content, kind string) *PropView {
	v := c.Body
	switch {
	case v == nil, !t.keeps(v):
		return nil
	case kind == bodyForm, kind == bodyMultipart:
		if v.Object == nil {
			return nil
		}
	case kind != bodyJSON:
		return nil
	}
	t.hasRoots = true
	return t.prop(v, false, "")
}

// view is the table, nil when no handler checks a body.
func (t *presenceTable) view() *PresenceView {
	if !t.hasRoots {
		return nil
	}
	objects := map[*gomodel.Decl]ObjectView{}
	// object adds the objects it names to used, so the loop reads used as it grows.
	for i := 0; i < len(t.used); i++ {
		objects[t.used[i]] = t.object(t.used[i])
	}

	// The runtime finds an object by binary search, so the order is that of the names unquoted.
	slices.SortFunc(t.used, func(a, b *gomodel.Decl) int { return strings.Compare(a.Name, b.Name) })
	v := &PresenceView{Runtime: t.runtime, IsChecked: t.isChecked}
	for _, d := range t.used {
		v.Objects = append(v.Objects, objects[d])
	}
	return v
}

func (t *presenceTable) object(d *gomodel.Decl) ObjectView {
	o := ObjectView{Name: gocode.Quote(d.Name), IsClosed: t.isChecked && d.Struct.IsClosed}
	fields := slices.SortedFunc(slices.Values(d.Struct.Fields), func(a, b *gomodel.Field) int { return strings.Compare(a.JSONName, b.JSONName) })
	for _, f := range fields {
		if f.Value == nil || !t.isChecked && f.Default == "" && !t.leads(f.Value) {
			continue
		}
		p := t.prop(f.Value, f.Required && !f.ReadOnly, f.Default)
		p.Key = gocode.Quote(f.JSONName)
		o.Props = append(o.Props, p)
	}

	if ap := d.Struct.AdditionalProperties; ap != nil && ap.Value != nil && t.keeps(ap.Value.Values) {
		o.Extra = t.prop(ap.Value.Values, false, "")
	}
	return o
}

func (t *presenceTable) prop(v *gomodel.BodyValue, isRequired bool, def string) *PropView {
	p := &PropView{Runtime: t.runtime, Default: quoteDefault(def)}
	if t.isChecked {
		p.IsRequired, p.IsNullable = isRequired, v.IsNullable
	}
	if d := v.Object; d != nil && (t.isChecked || t.hasDefault[d]) {
		p.Object = gocode.Quote(d.Name)
		if !t.isUsed[d] {
			t.isUsed[d] = true
			t.used = append(t.used, d)
		}
	}
	if t.keeps(v.Items) {
		p.Items = t.prop(v.Items, false, "")
	}
	if t.keeps(v.Values) {
		p.Values = t.prop(v.Values, false, "")
	}
	return p
}

// keeps reports a value the table writes: one that checks something, or leads to a default.
func (t *presenceTable) keeps(v *gomodel.BodyValue) bool {
	switch {
	case v == nil:
		return false
	case !t.isChecked:
		return t.leads(v)
	}
	return !v.IsNullable || v.Object != nil || t.keeps(v.Items) || t.keeps(v.Values)
}

// leads reports a value that holds a struct with a default, at any depth.
func (t *presenceTable) leads(v *gomodel.BodyValue) bool {
	return v != nil && (v.Object != nil && t.hasDefault[v.Object] || t.leads(v.Items) || t.leads(v.Values))
}

func (t *presenceTable) declLeads(d *gomodel.Decl) bool {
	for _, f := range d.Struct.Fields {
		if f.Default != "" || t.leads(f.Value) {
			return true
		}
	}
	ap := d.Struct.AdditionalProperties
	return ap != nil && ap.Value != nil && t.leads(ap.Value.Values)
}

// reach adds the structs v holds, at any depth, to decls.
func reach(v *gomodel.BodyValue, seen map[*gomodel.Decl]bool, decls []*gomodel.Decl) []*gomodel.Decl {
	if v == nil {
		return decls
	}
	if d := v.Object; d != nil && !seen[d] {
		seen[d] = true
		decls = append(decls, d)
		for _, f := range d.Struct.Fields {
			decls = reach(f.Value, seen, decls)
		}
		if ap := d.Struct.AdditionalProperties; ap != nil {
			decls = reach(ap.Value, seen, decls)
		}
	}
	return reach(v.Values, seen, reach(v.Items, seen, decls))
}
