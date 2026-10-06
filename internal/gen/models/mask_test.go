// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package models

import (
	"testing"

	"github.com/mockzilla/mockzilla-codegen/internal/extension"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

// TestViewRendersMasks compares with testdata/masks.golden. UPDATE=1 writes it instead.
func TestViewRendersMasks(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	digits := &gomodel.Pattern{Name: "patternUserSsn", Source: `\d`, Part: gomodel.PartTypes}
	contact := &gomodel.Decl{Name: "Contact", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Email", JSONName: "email", Type: str, Sensitive: &extension.Mask{}}},
	}, Masks: []*gomodel.Mask{{Field: "Email"}}}
	user := &gomodel.Decl{Name: "User", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{Fields: []*gomodel.Field{
		{Name: "Password", JSONName: "password", Type: gomodel.Pointer{Elem: str}},
		{Name: "Ssn", JSONName: "ssn", Type: str},
		{Name: "Token", JSONName: "token", Type: str},
		{Name: "Card", JSONName: "card", Type: str},
		{Name: "Pin", JSONName: "pin", Type: gomodel.Builtin{Name: "int"}},
		{Name: "Main", JSONName: "main", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: contact}}},
		{Name: "Plain", JSONName: "plain", Type: gomodel.DeclRef{Decl: contact}},
		{Name: "List", JSONName: "list", Type: gomodel.Slice{Elem: gomodel.DeclRef{Decl: contact}}},
		{Name: "ByName", JSONName: "byName", Type: gomodel.Map{Key: str, Elem: gomodel.DeclRef{Decl: contact}}},
		{Name: "Old", JSONName: "old", Type: str, Deprecated: true, DeprecatedReason: "Use Password."},
		{Name: "Hidden", JSONName: "hidden", Type: str, IsJSONIgnored: true},
		{Name: "Nick", JSONName: "nick", Type: gomodel.Nullable{Elem: str}},
		{Name: "Backup", JSONName: "backup", Type: gomodel.Nullable{Elem: gomodel.DeclRef{Decl: contact}}},
		{Name: "Others", JSONName: "others", Type: gomodel.Nullable{Elem: gomodel.Slice{Elem: gomodel.DeclRef{Decl: contact}}}},
		{Name: "Code", JSONName: "code", Type: gomodel.Nullable{Elem: gomodel.Builtin{Name: "int"}}},
	}}, Masks: []*gomodel.Mask{
		{Field: "Password", IsPointer: true},
		{Field: "Ssn", Kind: gomodel.MaskRegex, Pattern: digits},
		{Field: "Token", Kind: gomodel.MaskHash},
		{Field: "Card", Kind: gomodel.MaskPartial, KeepPrefix: 1, KeepSuffix: 4},
		{Field: "Pin", Kind: gomodel.MaskZero},
		{Field: "Main", Kind: gomodel.MaskNested, IsPointer: true},
		{Field: "Plain", Kind: gomodel.MaskNested},
		{Field: "List", Kind: gomodel.MaskItems},
		{Field: "ByName", Kind: gomodel.MaskValues},
		{Field: "Nick", IsWrapped: true},
		{Field: "Backup", Kind: gomodel.MaskNested, IsWrapped: true},
		{Field: "Others", Kind: gomodel.MaskItems, IsWrapped: true},
		{Field: "Code", Kind: gomodel.MaskZero, IsWrapped: true},
	}, Deprecated: true, DeprecatedReason: "Use Account."}
	contacts := &gomodel.Decl{Name: "Contacts", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: gomodel.Slice{Elem: gomodel.DeclRef{Decl: contact}}, Masks: []*gomodel.Mask{{Kind: gomodel.MaskItems}}}

	g := New(&gomodel.Model{Decls: []*gomodel.Decl{contact, user, contacts}, Patterns: []*gomodel.Pattern{digits}})
	checkRender(t, g, "masks")
}
