// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/naming"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/spec"
)

// structMethods are generated on structs, so fields cannot take these names.
var structMethods = []string{"Validate", "MarshalJSON", "UnmarshalJSON"}

// additionalMethods come with an AdditionalProperties field.
var additionalMethods = []string{"AdditionalProperties", "Get", "Set"}

// resolveOperations names operations, then webhooks. They become methods, so they have a scope
// of their own.
func resolveOperations(doc *spec.Document, n *naming.Namer, c *diag.Collector) []*Operation {
	all := slices.Concat(doc.Operations, doc.Webhooks)
	reqs := make([]naming.Request, len(all))
	for i, op := range all {
		reqs[i] = naming.Request{ID: op.Origin.Pointer, Want: n.Exported(op.ID), Rank: naming.RankOperation, Order: i, Origin: origin(op.Origin)}
	}
	res := resolve(nil, reqs, c)

	ops := make([]*Operation, len(all))
	for i, op := range all {
		ops[i] = &Operation{Name: res.Names[op.Origin.Pointer], Spec: op}
	}
	return ops
}

// resolveTypes names declarations in rounds by depth, so an inline name builds on the final name
// of its parent. Names from earlier rounds are reserved in later ones; a rename still reports the
// declaration that holds the name. It returns every name taken.
func resolveTypes(list []*pending, reserved []string, c *diag.Collector) []string {
	var rounds [][]*pending
	for _, p := range list {
		for len(rounds) <= p.depth {
			rounds = append(rounds, nil)
		}
		rounds[p.depth] = append(rounds[p.depth], p)
	}

	taken := slices.Clone(reserved)
	holders := map[string]string{}
	for _, round := range rounds {
		reqs := make([]naming.Request, len(round))
		for i, p := range round {
			want := p.name
			if p.base != nil {
				want = p.base.decl.Name + p.name
			}
			reqs[i] = naming.Request{ID: p.decl.ID, Want: want, Fallback: p.fallback, Rank: p.rank, Order: p.order, Origin: p.decl.Origin}
		}

		res := naming.Resolve(taken, reqs)
		for _, r := range res.Renames {
			if r.Holder == "" {
				r.Holder = holders[strings.ToLower(r.From)]
			}
			c.Append(r.Diagnostic())
		}
		for _, p := range round {
			p.decl.Name = res.Names[p.decl.ID]
		}
		for _, a := range res.Ordered {
			taken = append(taken, a.Name)
			holders[strings.ToLower(a.Name)] = a.ID
		}
	}
	return taken[len(reserved):]
}

// resolveFields names struct fields against the methods the struct gets.
func resolveFields(d *Decl, n *naming.Namer, c *diag.Collector) {
	reserved := structMethods
	if d.Struct.AdditionalProperties != nil {
		reserved = slices.Concat(structMethods, additionalMethods)
	}

	reqs := make([]naming.Request, len(d.Struct.Fields))
	for i, f := range d.Struct.Fields {
		reqs[i] = naming.Request{ID: fieldID(d, f), Want: n.Exported(f.JSONName), Rank: naming.RankInline, Order: i, Origin: f.Origin}
	}
	res := resolve(reserved, reqs, c)
	for _, f := range d.Struct.Fields {
		f.Name = res.Names[fieldID(d, f)]
	}
}

// resolveConstants names enum constants once every type has its name: a constant is named after
// its type and must not take a type's name.
func resolveConstants(decls []*Decl, reserved []string, opts Options, c *diag.Collector) {
	var reqs []naming.Request
	for _, d := range decls {
		if d.Kind != KindEnum {
			continue
		}
		for i, v := range d.Enum.Values {
			r := naming.Request{ID: constID(d, i), Want: opts.Namer.EnumConst(d.Name, valueText(v.Value)), Order: len(reqs), Origin: d.Origin}
			if !opts.EnumPrefix {
				r.Want, r.Fallback = opts.Namer.Exported(valueText(v.Value)), r.Want
			}
			reqs = append(reqs, r)
		}
	}

	res := resolve(reserved, reqs, c)
	for _, d := range decls {
		if d.Kind != KindEnum {
			continue
		}
		for i := range d.Enum.Values {
			d.Enum.Values[i].Name = res.Names[constID(d, i)]
		}
	}
}

func resolve(reserved []string, reqs []naming.Request, c *diag.Collector) naming.Result {
	res := naming.Resolve(reserved, reqs)
	for _, r := range res.Renames {
		c.Append(r.Diagnostic())
	}
	return res
}

func fieldID(d *Decl, f *Field) string {
	return d.ID + "/properties/" + oasdoc.Escape(f.JSONName)
}

func constID(d *Decl, i int) string {
	return d.ID + "/enum/" + strconv.Itoa(i)
}
