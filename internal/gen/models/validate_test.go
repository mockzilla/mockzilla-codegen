// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package models

import (
	"testing"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// TestViewRendersValidation compares with testdata/validation.golden. UPDATE=1 writes it instead.
func TestViewRendersValidation(t *testing.T) {
	t.Parallel()

	str := gomodel.Builtin{Name: "string"}
	num := gomodel.Builtin{Name: "float64"}
	code := &gomodel.Pattern{Name: "patternPetCode", Text: `^[A-Z].$`, Source: `^[A-Z][^\n\r\x{2028}\x{2029}]$`, Part: gomodel.PartTypes}
	owner := &gomodel.Decl{Name: "Owner", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "ID", JSONName: "id", Type: str}},
	}, Validation: &gomodel.Validation{HasResponse: true, Checks: []*gomodel.Check{
		{Field: "ID", Path: "id", Side: gomodel.SideResponse, Rules: []gomodel.Rule{{Kind: gomodel.RuleFormat, Format: "uuid"}}},
	}}}
	status := &gomodel.Decl{Name: "Status", Part: gomodel.PartEnums, Kind: gomodel.KindEnum, Enum: &gomodel.Enum{Base: str, Values: []gomodel.EnumValue{
		{Name: "StatusOn", Value: spec.Value{Kind: spec.KindString, Str: "on"}},
		{Name: "StatusOff", Value: spec.Value{Kind: spec.KindString, Str: "off"}},
	}}, Validation: &gomodel.Validation{}}
	empty := &gomodel.Decl{Name: "Empty", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Validation: &gomodel.Validation{}}
	pet := &gomodel.Decl{Name: "Pet", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{Fields: []*gomodel.Field{
		{Name: "Name", JSONName: "name", Type: str},
		{Name: "Code", JSONName: "code", Type: gomodel.Pointer{Elem: str}},
		{Name: "Weight", JSONName: "weight", Type: num},
		{Name: "Tags", JSONName: "tags", Type: gomodel.Slice{Elem: str}},
		{Name: "Grid", JSONName: "grid", Type: gomodel.Slice{Elem: gomodel.Slice{Elem: num}}},
		{Name: "Labels", JSONName: "labels", Type: gomodel.Map{Key: str, Elem: str}},
		{Name: "Owner", JSONName: "owner", Type: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: owner}}},
		{Name: "Secret", JSONName: "secret", Type: str},
		{Name: "Photo", JSONName: "photo", Type: gomodel.Slice{Elem: gomodel.Builtin{Name: "byte"}}},
		{Name: "Extra", JSONName: "extra", Type: gomodel.Map{Key: str, Elem: gomodel.Builtin{Name: "any"}}},
	}}, Validation: &gomodel.Validation{HasResponse: true, Checks: []*gomodel.Check{
		{Field: "Name", Path: "name", Rules: []gomodel.Rule{
			{Kind: gomodel.RuleMinLength, Number: "1"},
			{Kind: gomodel.RuleMaxLength, Number: "9"},
			{Kind: gomodel.RuleConst, Const: spec.Value{Kind: spec.KindString, Str: "Rex"}},
		}},
		{Field: "Code", Path: "code", IsPointer: true, IsGuarded: true, Rules: []gomodel.Rule{{Kind: gomodel.RulePattern, Pattern: code}}},
		{Field: "Weight", Path: "weight", Rules: []gomodel.Rule{
			{Kind: gomodel.RuleMinimum, Number: "0", IsExclusive: true},
			{Kind: gomodel.RuleMaximum, Number: "9.5"},
			{Kind: gomodel.RuleMultipleOf, Number: "0.5"},
		}},
		{Field: "Tags", Path: "tags", IsGuarded: true, IsRequired: true, Rules: []gomodel.Rule{
			{Kind: gomodel.RuleMinItems, Number: "1"},
			{Kind: gomodel.RuleMaxItems, Number: "3"},
			{Kind: gomodel.RuleUnique},
		}},
		{Field: "Grid", Path: "grid", IsGuarded: true, Rules: []gomodel.Rule{{Kind: gomodel.RuleUniqueJSON}}, Items: &gomodel.Check{
			IsGuarded: true,
			Items:     &gomodel.Check{Rules: []gomodel.Rule{{Kind: gomodel.RuleMinimum, Number: "0"}}},
		}},
		{Field: "Labels", Path: "labels", IsGuarded: true, Rules: []gomodel.Rule{
			{Kind: gomodel.RuleMinProperties, Number: "1"},
			{Kind: gomodel.RuleMaxProperties, Number: "2"},
		}, Values: &gomodel.Check{Rules: []gomodel.Rule{{Kind: gomodel.RuleMinLength, Number: "1"}}}, Keys: []gomodel.Rule{
			{Kind: gomodel.RulePattern, Pattern: code},
			{Kind: gomodel.RuleEnum, Values: []spec.Value{{Kind: spec.KindString, Str: "A"}, {Kind: spec.KindString, Str: "B"}}},
		}},
		{Field: "Owner", Path: "owner", IsPointer: true, IsGuarded: true, IsNested: true, Nested: owner},
		{Field: "Secret", Path: "secret", Side: gomodel.SideRequest, Rules: []gomodel.Rule{{Kind: gomodel.RuleMinLength, Number: "8"}}},
		{Field: "Photo", Path: "photo", IsGuarded: true, Rules: []gomodel.Rule{
			{Kind: gomodel.RuleMaxLength, Number: "8", IsBase64: true},
			{Kind: gomodel.RulePattern, Pattern: code, IsBase64: true},
		}},
		{Field: "Extra", Path: "extra", Keys: []gomodel.Rule{{Kind: gomodel.RuleMaxLength, Number: "4"}}},
	}}}
	pets := &gomodel.Decl{Name: "Pets", Part: gomodel.PartTypes, Kind: gomodel.KindDefined, Target: gomodel.Slice{Elem: gomodel.DeclRef{Decl: pet}}, Validation: &gomodel.Validation{
		HasResponse: true,
		Checks: []*gomodel.Check{{
			Rules: []gomodel.Rule{{Kind: gomodel.RuleMaxItems, Number: "2"}},
			Items: &gomodel.Check{IsNested: true, Nested: pet},
		}},
	}}
	statusVariant := &gomodel.Variant{Name: "Status", FieldType: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: status}}, Kinds: gomodel.JSONString}
	numberVariant := &gomodel.Variant{Name: "Number", FieldType: gomodel.Pointer{Elem: num}, Kinds: gomodel.JSONNumber}
	choice := &gomodel.Decl{Name: "Choice", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, Struct: &gomodel.Struct{}, Union: groupUnion(gomodel.Group{
		Variants: []*gomodel.Variant{statusVariant, numberVariant},
	}), Validation: &gomodel.Validation{Counts: []gomodel.Count{{Func: "ExactlyOne", Variants: []*gomodel.Variant{statusVariant, numberVariant}}}, Checks: []*gomodel.Check{
		{Field: "Status", IsPointer: true, IsGuarded: true, IsNested: true, Nested: status},
	}}, Error: &gomodel.ErrorMessage{Path: "detail"}}
	ownerVariant := &gomodel.Variant{Name: "Owner", FieldType: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: owner}}, Kinds: gomodel.JSONObject, Values: []string{"owner"}}
	emptyVariant := &gomodel.Variant{Name: "Empty", FieldType: gomodel.Pointer{Elem: gomodel.DeclRef{Decl: empty}}, Kinds: gomodel.JSONObject, Values: []string{"empty"}}
	tagged := &gomodel.Decl{Name: "Tagged", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, Struct: &gomodel.Struct{}, Union: groupUnion(gomodel.Group{
		Discriminator: "kind",
		Variants:      []*gomodel.Variant{ownerVariant, emptyVariant},
	}), Validation: &gomodel.Validation{Counts: []gomodel.Count{{Func: "ExactlyOne", Variants: []*gomodel.Variant{ownerVariant, emptyVariant}}}, IsDiscriminated: true}}
	eitherVariants := []*gomodel.Variant{
		{Name: "Number", FieldType: gomodel.Pointer{Elem: num}, Kinds: gomodel.JSONNumber},
		{Name: "Code", FieldType: gomodel.Pointer{Elem: str}, Kinds: gomodel.JSONString},
		{Name: "Tags", FieldType: gomodel.Slice{Elem: str}, Kinds: gomodel.JSONArray},
	}
	either := &gomodel.Decl{Name: "Either", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, Struct: &gomodel.Struct{}, Union: groupUnion(gomodel.Group{
		IsAnyOf:  true,
		Variants: eitherVariants,
	}), Validation: &gomodel.Validation{Counts: []gomodel.Count{{Func: "AtLeastOne", Variants: eitherVariants}}, Checks: []*gomodel.Check{
		{Field: "Code", IsVariant: true, IsPointer: true, IsGuarded: true, Rules: []gomodel.Rule{{Kind: gomodel.RuleMaxLength, Number: "5"}}},
		{Field: "Tags", IsVariant: true, IsGuarded: true, Rules: []gomodel.Rule{{Kind: gomodel.RuleMinItems, Number: "1"}}, Items: &gomodel.Check{
			Rules: []gomodel.Rule{{Kind: gomodel.RuleMinLength, Number: "1"}},
		}},
	}}}
	first := &gomodel.Variant{Name: "First", FieldType: gomodel.Pointer{Elem: str}, Kinds: gomodel.JSONString}
	shared := &gomodel.Variant{Name: "Shared", FieldType: gomodel.Pointer{Elem: str}, Kinds: gomodel.JSONString}
	last := &gomodel.Variant{Name: "Last", FieldType: gomodel.Pointer{Elem: str}, Kinds: gomodel.JSONString}
	plain := &gomodel.Variant{Name: "Plain", FieldType: gomodel.Pointer{Elem: num}, Kinds: gomodel.JSONNumber}
	other := &gomodel.Variant{Name: "Other", FieldType: gomodel.Pointer{Elem: num}, Kinds: gomodel.JSONNumber}
	mixed := &gomodel.Decl{Name: "Mixed", Part: gomodel.PartUnions, Kind: gomodel.KindUnion, Struct: &gomodel.Struct{}, Union: &gomodel.Union{
		Variants: []*gomodel.Variant{first, shared, last, plain, other},
		Groups: []*gomodel.Group{
			{Variants: []*gomodel.Variant{first, shared}},
			{IsAnyOf: true, Variants: []*gomodel.Variant{shared, last}},
			{IsAnyOf: true, Variants: []*gomodel.Variant{plain, other}},
		},
	}, Validation: &gomodel.Validation{
		Counts: []gomodel.Count{
			{Func: "ExactlyOne", Variants: []*gomodel.Variant{first, shared}},
			{Func: "AtLeastOne", Variants: []*gomodel.Variant{shared, last}},
			{Func: "AtLeastOne", Variants: []*gomodel.Variant{plain, other}},
		},
		Checks: []*gomodel.Check{
			{Field: "Shared", IsVariant: true, IsPointer: true, IsGuarded: true, Rules: []gomodel.Rule{{Kind: gomodel.RuleMaxLength, Number: "5"}}},
			{Field: "Last", IsVariant: true, IsPointer: true, IsGuarded: true, Rules: []gomodel.Rule{{Kind: gomodel.RuleMinLength, Number: "2"}}},
		},
	}}
	quoted := spec.Value{Kind: spec.KindObject, Fields: []spec.Field{{Name: "a", Value: spec.Value{Kind: spec.KindString, Str: "x`y"}}}}
	corner := &gomodel.Decl{Name: "Corner", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{}, Validation: &gomodel.Validation{Checks: []*gomodel.Check{
		{Rules: []gomodel.Rule{{Kind: gomodel.RuleEnumJSON, Values: []spec.Value{{Kind: spec.KindArray, Items: []spec.Value{{Kind: spec.KindNumber, Num: "1"}}}, quoted}}}},
	}}}
	patch := &gomodel.Decl{Name: "Patch", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{Fields: []*gomodel.Field{
		{Name: "Nick", JSONName: "nick", Type: gomodel.Nullable{Elem: str}, OmitEmpty: true, OmitZero: true},
		{Name: "Boss", JSONName: "boss", Type: gomodel.Nullable{Elem: gomodel.DeclRef{Decl: owner}}, OmitEmpty: true, OmitZero: true},
		{Name: "Scores", JSONName: "scores", Type: gomodel.Slice{Elem: gomodel.Nullable{Elem: num}}},
		{Name: "Names", JSONName: "names", Type: gomodel.Nullable{Elem: gomodel.Slice{Elem: str}}, OmitEmpty: true, OmitZero: true},
	}}, Validation: &gomodel.Validation{HasResponse: true, Checks: []*gomodel.Check{
		{Field: "Nick", Path: "nick", IsWrapped: true, IsGuarded: true, IsNullRejected: true, Rules: []gomodel.Rule{{Kind: gomodel.RuleMinLength, Number: "1"}}},
		{Field: "Boss", Path: "boss", IsWrapped: true, IsGuarded: true, IsNested: true, Nested: owner},
		{Field: "Scores", Path: "scores", IsGuarded: true, Items: &gomodel.Check{
			IsWrapped: true, IsGuarded: true, Rules: []gomodel.Rule{{Kind: gomodel.RuleMinimum, Number: "0"}},
		}},
		{Field: "Names", Path: "names", IsWrapped: true, IsGuarded: true, Items: &gomodel.Check{Rules: []gomodel.Rule{{Kind: gomodel.RuleMinLength, Number: "1"}}}},
	}}}
	problem := &gomodel.Decl{Name: "Problem", Part: gomodel.PartTypes, Kind: gomodel.KindStruct, Struct: &gomodel.Struct{
		Fields: []*gomodel.Field{{Name: "Message", JSONName: "message", Type: str}},
	}, Error: &gomodel.ErrorMessage{Path: "message", HasConstructor: true}}

	g := New(&gomodel.Model{
		Decls:    []*gomodel.Decl{owner, status, empty, pet, pets, choice, tagged, either, mixed, corner, patch, problem},
		Patterns: []*gomodel.Pattern{code},
	})
	checkRender(t, g, "validation")
}

func TestRuleFuncs(t *testing.T) {
	t.Parallel()

	for kind := gomodel.RuleMinLength; kind <= gomodel.RuleEnumJSON; kind++ {
		if ruleFuncs[kind] == "" {
			t.Errorf("rule kind %d has no runtime function", kind)
		}
	}
}
