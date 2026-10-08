// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Which declarations a form body or a deepObject parameter holds that read a form themselves.

package gomodel

import "slices"

// markForms sets IsForm on what a form body or a deepObject parameter holds, at any depth.
func markForms(ops []*Operation) {
	seen := map[*Decl]bool{}
	for _, op := range ops {
		for _, g := range op.Params {
			markDeep(g, seen)
		}
		contents := slices.Clone(op.Bodies)
		for _, r := range op.Responses {
			contents = append(contents, r.Contents...)
		}
		for _, c := range contents {
			switch baseMediaType(c.MediaType) {
			case "application/x-www-form-urlencoded", "multipart/form-data":
				markForm(c.Type, seen)
			}
		}
	}
}

// markDeep marks what the deepObject parameters of g hold: their bracketed keys read like a form's.
func markDeep(g ParamGroup, seen map[*Decl]bool) {
	for i, f := range g.Decl.Struct.Fields {
		if g.Params[i].Style == styleDeep {
			markForm(f.Type, seen)
		}
	}
}

// markForm marks the declarations a value of type t holds.
func markForm(t Type, seen map[*Decl]bool) {
	switch x := t.(type) {
	case Pointer:
		markForm(x.Elem, seen)
	case Nullable:
		markForm(x.Elem, seen)
	case Slice:
		markForm(x.Elem, seen)
	case Map:
		markForm(x.Elem, seen)
	case DeclRef:
		d := x.Decl
		if seen[d] {
			return
		}
		seen[d] = true

		switch {
		case d.Union != nil:
			// Bracketed keys reach an object or a list: address[city], items[0][price].
			d.IsForm = slices.ContainsFunc(d.Union.Variants, func(v *Variant) bool { return v.Kinds == 0 || v.Kinds&(JSONObject|JSONArray) != 0 })
			for _, v := range d.Union.Variants {
				markForm(v.Type, seen)
			}
		case d.Struct != nil:
			if ap := d.Struct.AdditionalProperties; ap != nil {
				_, d.IsForm = ap.Type.(Map)
				markForm(ap.Type, seen)
			}
		case d.Target != nil:
			markForm(d.Target, seen)
		}

		if d.Struct != nil {
			for _, f := range d.Struct.Fields {
				markForm(f.Type, seen)
			}
		}
	}
}
