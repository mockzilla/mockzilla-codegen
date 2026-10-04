// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Names for operations, types, fields, variants and constants, with clashes settled.

package gomodel

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/extension"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// structMethods are generated on structs, so fields cannot take these names.
var structMethods = []string{"Validate", "MarshalJSON", "UnmarshalJSON", "Masked", "LogValue"}

// additionalMethods come with an AdditionalProperties field.
var additionalMethods = []string{"AdditionalProperties", "Get", "Set"}

// textMethods come with a union of scalars, so its variants cannot take these names.
var textMethods = []string{"MarshalText", "UnmarshalText"}

// Methods are the suffixes of the methods the generator declares next to the one named after an
// operation. Every operation but a webhook gets Client; one that answers a 2xx in a sequential
// media type also gets Stream, and one the MCP tools keep gets Tool, where x-mcp decides over
// IsToolSkipped.
type Methods struct {
	Client        []string
	Stream        []string
	Tool          []string
	IsToolSkipped bool
}

// of lists the suffixes of op's methods. What is wrong in its x-mcp is reported by the MCP
// generator.
func (m Methods) of(op *spec.Operation) []string {
	if op.IsWebhook {
		return nil
	}

	out := slices.Clone(m.Client)
	if hasStream(op) {
		out = append(out, m.Stream...)
	}
	if set, _ := extension.Parse(op.Extensions, op.Origin); !set.MCP.IsSkipped(m.IsToolSkipped) {
		out = append(out, m.Tool...)
	}
	return out
}

// resolveOperations names operations, then webhooks. They become methods, so they have a scope
// of their own, in which reserved are the methods the generator declares. An operation holds the
// names of its other methods too, so of getCert and getCertRequest the second is renamed: the
// first has a method getCertRequest.
func resolveOperations(doc *spec.Document, opts Options, c *diag.Collector) []*Operation {
	all := slices.Concat(doc.Operations, doc.Webhooks)
	reqs := make([]naming.Request, len(all))
	for i, op := range all {
		reqs[i] = naming.Request{
			ID:      op.Origin.Pointer,
			Want:    opts.Namer.Exported(op.ID),
			Methods: opts.Methods.of(op),
			Rank:    naming.RankOperation,
			Order:   i,
			Origin:  origin(op.Origin),
		}
	}
	res := resolve(opts.ReservedOperations, reqs, c)

	ops := make([]*Operation, len(all))
	for i, op := range all {
		ops[i] = &Operation{Name: res.Names[op.Origin.Pointer], Spec: op}
	}
	return ops
}

// hasStream reports an operation that answers a 2xx in a sequential media type, which the client
// reads with a Stream method.
func hasStream(op *spec.Operation) bool {
	return slices.ContainsFunc(op.Responses, func(r *spec.Response) bool {
		code, ok := spec.StatusCode(r.Status)
		return ok && code >= 200 && code <= 299 && slices.ContainsFunc(r.Contents, func(mt *spec.MediaType) bool {
			return runtime.IsSequential(mt.Name)
		})
	})
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

// resolveFields names struct fields against methods, the methods the struct gets.
func resolveFields(d *Decl, n *naming.Namer, methods []string, c *diag.Collector) {
	reserved := methods
	if d.Struct.AdditionalProperties != nil {
		reserved = slices.Concat(methods, additionalMethods)
	}

	reqs := make([]naming.Request, len(d.Struct.Fields))
	for i, f := range d.Struct.Fields {
		reqs[i] = naming.Request{ID: fieldID(d, f), Want: n.Exported(f.JSONName), Rank: naming.RankInline, Order: i, Origin: f.Origin}
		if f.goName != "" {
			reqs[i].Want, reqs[i].Rank = f.goName, naming.RankGoName
		}
	}
	res := resolve(reserved, reqs, c)
	for _, f := range d.Struct.Fields {
		f.Name = res.Names[fieldID(d, f)]
	}
}

// resolveVariants names the fields of union variants after the shared fields, which keep theirs.
func resolveVariants(d *Decl, methods []string, c *diag.Collector) {
	reserved := slices.Concat(methods, textMethods)
	for _, f := range d.Struct.Fields {
		reserved = append(reserved, f.Name)
	}

	reqs := make([]naming.Request, len(d.Union.Variants))
	for i, v := range d.Union.Variants {
		reqs[i] = naming.Request{ID: variantID(d, i), Want: v.Name, Rank: naming.RankInline, Order: i, Origin: v.Origin}
	}
	res := resolve(reserved, reqs, c)
	for i, v := range d.Union.Variants {
		v.Name = res.Names[variantID(d, i)]
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
			switch {
			case i < len(d.enumNames):
				r.Want, r.Rank = d.enumNames[i], naming.RankGoName
			case !opts.EnumPrefix:
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

func variantID(d *Decl, i int) string {
	return d.ID + "/variants/" + strconv.Itoa(i)
}

func constID(d *Decl, i int) string {
	return d.ID + "/enum/" + strconv.Itoa(i)
}
