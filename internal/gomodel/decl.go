// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

type DeclKind int

const (
	KindStruct DeclKind = iota
	// KindAlias is type X = Y.
	KindAlias
	// KindDefined is type X Y.
	KindDefined
	KindEnum
	// KindUnion is a struct with a field per variant, next to the fields its variants share.
	KindUnion
)

// JSONKind is a set of JSON value kinds, as runtime.Kind has them.
type JSONKind uint8

const (
	JSONNull JSONKind = 1 << iota
	JSONBool
	JSONInteger
	JSONNumber
	JSONString
	JSONArray
	JSONObject

	JSONAny = JSONNull | JSONBool | JSONInteger | JSONNumber | JSONString | JSONArray | JSONObject
)

// Parts a declaration goes to. Layout places each part in one file.
const (
	PartTypes     = "models.types"
	PartEnums     = "models.enums"
	PartUnions    = "models.unions"
	PartParams    = "models.params"
	PartBodies    = "models.bodies"
	PartResponses = "models.responses"
)

// Decl is one package-level type. ID is the JSON pointer it comes from. Struct is set for
// KindStruct and KindUnion, Union for KindUnion, Enum for KindEnum, Target for the other kinds.
type Decl struct {
	ID         string
	Name       string
	Part       string
	Kind       DeclKind
	Doc        string
	Deprecated bool
	Struct     *Struct
	Union      *Union
	Enum       *Enum
	Target     Type
	Origin     diag.Origin
}

// Struct lists its fields in spec order. AdditionalProperties, when set, holds the keys that are
// not properties: its type is a map and its JSON name is "-". IsClosed is additionalProperties:
// false.
type Struct struct {
	Fields               []*Field
	AdditionalProperties *Field
	IsClosed             bool
}

// Union holds one field per variant. Exactly one is set for oneOf, one or more for anyOf; with
// IsNullable nothing set is null. Discriminator is the property whose value picks a variant.
type Union struct {
	IsAnyOf       bool
	IsNullable    bool
	Discriminator string
	Variants      []*Variant
}

// Variant is one member of a union. FieldType is a pointer to Type unless Type can be nil. The
// other fields tell decoding which JSON picks it; Known is nil when any key fits.
type Variant struct {
	Name      string
	Type      Type
	FieldType Type
	Values    []string
	IsDefault bool
	Kinds     JSONKind
	Required  []string
	Known     []string
	IsClosed  bool
	Origin    diag.Origin
}

// Field is one struct field. Required, Nullable, ReadOnly and WriteOnly repeat the spec.
type Field struct {
	Name       string
	JSONName   string
	Type       Type
	Required   bool
	Nullable   bool
	OmitEmpty  bool
	ReadOnly   bool
	WriteOnly  bool
	Deprecated bool
	Doc        string
	Tags       []Tag
	Origin     diag.Origin
}

// Tag is a struct tag written next to json, such as yaml:"name,omitempty".
type Tag struct {
	Key   string
	Value string
}

// Enum is a named type over Base with one constant per value, in spec order.
type Enum struct {
	Base   Type
	Values []EnumValue
}

type EnumValue struct {
	Name  string
	Value spec.Value
}

func origin(o spec.Origin) diag.Origin {
	return diag.Origin{File: o.File, Line: o.Line, Col: o.Col}
}
