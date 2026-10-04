// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Which declarations hold an x-sensitive-data value, and how their Masked method hides it.

package gomodel

import (
	"github.com/mockzilla/mockzilla-codegen/internal/extension"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// maskable is a value a declaration holds: a field, a union variant, or the value of a named slice
// or map type, which has no name.
type maskable struct {
	name      string
	typ       Type
	sensitive *extension.Mask
	at        spec.Origin
}

// planMasks gives Masks to every declaration that holds a sensitive value, itself or through the
// types it holds, loops of them included.
func planMasks(decls []*Decl, patterns *patternSet) {
	masked := map[*Decl]bool{}
	for isChanged := true; isChanged; {
		isChanged = false
		for _, d := range decls {
			if !masked[d] && holdsSensitive(d, masked) {
				masked[d] = true
				isChanged = true
			}
		}
	}

	for _, d := range decls {
		if !masked[d] {
			continue
		}
		for _, m := range maskables(d) {
			if mask := maskOf(d, m, masked, patterns); mask != nil {
				d.Masks = append(d.Masks, mask)
			}
		}
	}
}

func holdsSensitive(d *Decl, masked map[*Decl]bool) bool {
	for _, m := range maskables(d) {
		if _, isHeld := heldMask(m.typ, masked); m.sensitive != nil || isHeld {
			return true
		}
	}
	return false
}

func maskables(d *Decl) []maskable {
	var out []maskable
	switch d.Kind {
	case KindStruct, KindUnion:
		for _, f := range d.Struct.Fields {
			m := maskable{name: f.Name, typ: f.Type, sensitive: f.Sensitive}
			if f.schema != nil {
				m.at = f.schema.Origin
			}
			out = append(out, m)
		}
		if d.Union != nil {
			for _, v := range d.Union.Variants {
				out = append(out, maskable{name: v.Name, typ: v.FieldType})
			}
		}
	case KindDefined:
		out = append(out, maskable{typ: d.Target})
	case KindAlias, KindEnum:
	}
	return out
}

// maskOf is what Masked does to m, nil for nothing. A sensitive string is masked as asked, with a
// full mask when its pattern cannot be used; any other sensitive value is cleared.
func maskOf(d *Decl, m maskable, masked map[*Decl]bool, patterns *patternSet) *Mask {
	out := &Mask{Field: m.name, IsPointer: isPointer(m.typ)}
	s := m.sensitive
	switch {
	case s != nil && groupOf(Elem(m.typ)) == groupString:
		out.Kind, out.KeepPrefix, out.KeepSuffix = maskKind(s.Kind), s.KeepPrefix, s.KeepSuffix
		if s.Kind == extension.MaskRegex {
			if out.Pattern = patterns.add(d.Part, s.Pattern, m.at, d.Name+m.name); out.Pattern == nil {
				out.Kind = MaskFull
			}
		}
	case s != nil:
		out.Kind = MaskZero
	default:
		kind, isHeld := heldMask(m.typ, masked)
		if !isHeld {
			return nil
		}
		out.Kind = kind
	}
	return out
}

// heldMask is how a value of type t reaches a masked declaration: by value or pointer, as the items
// of a slice, or as the values of a map.
func heldMask(t Type, masked map[*Decl]bool) (MaskKind, bool) {
	isMasked := func(t Type) bool {
		r, ok := unalias(t).(DeclRef)
		return ok && masked[r.Decl]
	}
	switch u := unalias(Elem(t)).(type) {
	case Slice:
		return MaskItems, isMasked(u.Elem)
	case Map:
		return MaskValues, isMasked(u.Elem)
	}
	return MaskNested, isMasked(Elem(t))
}

func maskKind(k extension.MaskKind) MaskKind {
	switch k {
	case extension.MaskRegex:
		return MaskRegex
	case extension.MaskHash:
		return MaskHash
	case extension.MaskPartial:
		return MaskPartial
	case extension.MaskFull:
	}
	return MaskFull
}
