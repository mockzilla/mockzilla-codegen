// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"path"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

var kindWords = map[DeclKind]string{KindStruct: "struct", KindAlias: "alias", KindDefined: "defined", KindEnum: "enum", KindUnion: "union"}

var jsonWords = []string{"null", "boolean", "integer", "number", "string", "array", "object"}

// Dump writes a model as plain text for tests and debugging. It is not Go source.
func Dump(m *Model) string {
	var b strings.Builder
	for _, d := range m.Decls {
		dumpDecl(&b, d)
	}
	for _, op := range m.Operations {
		dumpOperation(&b, op)
	}
	return b.String()
}

func dumpDecl(b *strings.Builder, d *Decl) {
	b.WriteString(d.Name + " " + kindWords[d.Kind])
	switch {
	case d.Enum != nil:
		b.WriteString(" " + typeText(d.Enum.Base))
	case d.Target != nil:
		b.WriteString(" " + typeText(d.Target))
	}
	b.WriteString(" " + d.Part)
	if d.Deprecated {
		b.WriteString(" deprecated")
	}
	if d.Doc != "" {
		b.WriteString(" doc=" + strconv.Quote(d.Doc))
	}
	b.WriteString("\n")

	if d.Enum != nil {
		for _, v := range d.Enum.Values {
			b.WriteString("  " + v.Name + " " + valueLiteral(v.Value) + "\n")
		}
	}
	if d.Struct != nil {
		for _, f := range d.Struct.Fields {
			dumpField(b, f)
		}
		if f := d.Struct.AdditionalProperties; f != nil {
			dumpField(b, f)
		}
	}
	if d.Union != nil {
		dumpUnion(b, d.Union)
	}
}

func dumpUnion(b *strings.Builder, u *Union) {
	b.WriteString("  |")
	for _, flag := range []struct {
		isSet bool
		word  string
	}{
		{!u.IsAnyOf, "oneOf"},
		{u.IsAnyOf, "anyOf"},
		{u.IsNullable, "nullable"},
		{u.Discriminator != "", "discriminator=" + u.Discriminator},
	} {
		if flag.isSet {
			b.WriteString(" " + flag.word)
		}
	}
	b.WriteString("\n")

	for _, v := range u.Variants {
		b.WriteString("  | " + v.Name + " " + typeText(v.FieldType) + " kinds=" + kindsText(v.Kinds))
		if len(v.Values) > 0 {
			b.WriteString(" values=" + strings.Join(v.Values, ","))
		}
		if v.IsDefault {
			b.WriteString(" default")
		}
		if len(v.Required) > 0 {
			b.WriteString(" required=" + strings.Join(v.Required, ","))
		}
		if v.Known != nil {
			b.WriteString(" known=" + strings.Join(v.Known, ","))
		}
		if v.IsClosed {
			b.WriteString(" closed")
		}
		b.WriteString("\n")
	}
}

func kindsText(k JSONKind) string {
	if k == JSONAny {
		return "any"
	}
	var words []string
	for i, w := range jsonWords {
		if k&(1<<i) != 0 {
			words = append(words, w)
		}
	}
	return strings.Join(words, "|")
}

func dumpField(b *strings.Builder, f *Field) {
	b.WriteString("  " + f.Name + " " + typeText(f.Type) + " json=" + f.JSONName)
	for _, flag := range []struct {
		isSet bool
		word  string
	}{
		{f.Required, "required"},
		{f.Nullable, "nullable"},
		{f.OmitEmpty, "omitempty"},
		{f.ReadOnly, "readOnly"},
		{f.WriteOnly, "writeOnly"},
		{f.Deprecated, "deprecated"},
	} {
		if flag.isSet {
			b.WriteString(" " + flag.word)
		}
	}
	for _, t := range f.Tags {
		b.WriteString(" " + t.Key + "=" + t.Value)
	}
	if f.Doc != "" {
		b.WriteString(" doc=" + strconv.Quote(f.Doc))
	}
	b.WriteString("\n")
}

func dumpOperation(b *strings.Builder, op *Operation) {
	b.WriteString("op " + op.Name + " " + op.Spec.Method + " " + op.Spec.Path + "\n")
	for _, p := range op.Params {
		b.WriteString("  params " + p.In + " " + p.Decl.Name + "\n")
	}
	for _, c := range op.Bodies {
		b.WriteString("  body " + c.MediaType + " " + typeText(c.Type) + "\n")
	}
	for _, r := range op.Responses {
		for _, c := range r.Contents {
			b.WriteString("  response " + r.Status + " " + c.MediaType + " " + typeText(c.Type) + "\n")
		}
	}
}

func typeText(t Type) string {
	switch t := t.(type) {
	case Builtin:
		return t.Name
	case DeclRef:
		return t.Decl.Name
	case Qualified:
		if t.Import.Alias != "" {
			return t.Import.Alias + "." + t.Name
		}
		return path.Base(t.Import.Path) + "." + t.Name
	case Pointer:
		return "*" + typeText(t.Elem)
	case Slice:
		return "[]" + typeText(t.Elem)
	case Map:
		return "map[" + typeText(t.Key) + "]" + typeText(t.Elem)
	}
	return "-"
}

func valueLiteral(v spec.Value) string {
	if v.Kind == spec.KindString {
		return strconv.Quote(v.Str)
	}
	return valueText(v)
}
