// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/spec"
)

type DeclKind int

const (
	KindStruct DeclKind = iota
	// KindAlias is type X = Y.
	KindAlias
	// KindDefined is type X Y.
	KindDefined
	KindEnum
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
// KindStruct, Enum for KindEnum, Target for the other kinds.
type Decl struct {
	ID         string
	Name       string
	Part       string
	Kind       DeclKind
	Doc        string
	Deprecated bool
	Struct     *Struct
	Enum       *Enum
	Target     Type
	Origin     diag.Origin
}

// Struct lists its fields in spec order. AdditionalProperties, when set, holds the keys that are
// not properties: its type is a map and its JSON name is "-".
type Struct struct {
	Fields               []*Field
	AdditionalProperties *Field
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
