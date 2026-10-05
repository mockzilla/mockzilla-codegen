// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// What the server checks and fills in a request body before it decodes it.

package gomodel

import (
	"cmp"

	"github.com/mockzilla/mockzilla-codegen/internal/jsonschema"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// BodyValue is what the server checks of one value of a request body; unions are not looked into.
type BodyValue struct {
	IsNullable bool
	Object     *Decl
	Items      *BodyValue
	Values     *BodyValue
}

// planBodies sets what the server checks of each request body and of the structs it holds.
func (b *builder) planBodies(ops []*Operation) {
	c := schemaChain{flat: b.flat, decls: b.decls}
	seen := map[*Decl]bool{}
	for _, op := range ops {
		if op.Spec.Body == nil || op.Spec.IsWebhook {
			continue
		}
		for i, mt := range op.Spec.Body.Contents {
			if mt.Schema != nil {
				op.Bodies[i].Body = b.bodyValue(c, mt.Schema, op.Bodies[i].Type, seen)
			}
		}
	}
}

// bodyValue is what the server checks of a value of type t, described by s.
func (b *builder) bodyValue(c schemaChain, s *spec.Schema, t Type, seen map[*Decl]bool) *BodyValue {
	v := &BodyValue{IsNullable: b.mayBeNull(s, t)}
	t = Elem(t)
	for {
		r, isRef := t.(DeclRef)
		if !isRef || r.Decl.Kind != KindAlias && r.Decl.Kind != KindDefined {
			break
		}
		s, t = cmp.Or(r.Decl.schema, s), Elem(r.Decl.Target)
	}

	switch u := t.(type) {
	case DeclRef:
		if u.Decl.Kind == KindStruct {
			v.Object = u.Decl
			b.planBodyStruct(c, u.Decl, seen)
		}
	case Slice:
		if u.Elem != byteType {
			v.Items = b.bodyValue(c, c.itemsOf(s), u.Elem, seen)
		}
	case Map:
		v.Values = b.bodyValue(c, c.valuesOf(s), u.Elem, seen)
	}
	return v
}

func (b *builder) planBodyStruct(c schemaChain, d *Decl, seen map[*Decl]bool) {
	if seen[d] {
		return
	}
	seen[d] = true

	for _, f := range d.Struct.Fields {
		f.Value = b.bodyValue(c, f.schema, f.Type, seen)
		if f.def != nil && !f.ReadOnly {
			f.Default = string(jsonschema.Marshal(*f.def))
		}
	}
	if ap := d.Struct.AdditionalProperties; ap != nil {
		ap.Value = b.bodyValue(c, d.schema, ap.Type, seen)
	}
}

// mayBeNull reports whether null fits: the schema allows it or, like properties alone, has no type.
func (b *builder) mayBeNull(s *spec.Schema, t Type) bool {
	under := Underlying(Elem(t))
	if s == nil || under == anyType || under == rawJSON || b.nullable(s) {
		return true
	}
	return !b.inChain(s, func(x *spec.Schema) bool {
		return x.Types != 0 || len(x.OneOf) > 0 || len(x.AnyOf) > 0 || len(x.Enum) > 0 || x.Const != nil
	})
}
