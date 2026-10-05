// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// What the model declares: structs, unions, enums and other named types, with their fields,
// checks and masks.

package gomodel

import (
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/extension"
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

	JSONAny    = JSONNull | JSONBool | JSONInteger | JSONNumber | JSONString | JSONArray | JSONObject
	JSONScalar = JSONBool | JSONInteger | JSONNumber | JSONString
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

// Side is who carries a value: readOnly values only come in responses, writeOnly ones only in
// requests.
type Side int

const (
	SideBoth Side = iota
	SideResponse
	SideRequest
)

// RuleKind picks the runtime check a rule calls.
type RuleKind int

const (
	RuleMinLength RuleKind = iota
	RuleMaxLength
	RulePattern
	RuleFormat
	RuleMinimum
	RuleMaximum
	RuleMultipleOf
	RuleMinItems
	RuleMaxItems
	RuleUnique
	RuleUniqueJSON
	RuleMinProperties
	RuleMaxProperties
	RuleConst
	RuleEnum
	RuleEnumJSON
)

// Decl is one package-level type. ID is the JSON pointer it comes from. Struct is set for
// KindStruct and KindUnion, Union for KindUnion, Enum for KindEnum, Target for the other kinds.
// Validation is nil for aliases and when validation is off; Error is set for error types; Masks is
// set for types that hold sensitive values. IsForm marks the unions and objects a form body holds.
type Decl struct {
	ID               string
	Name             string
	Part             string
	Kind             DeclKind
	Doc              string
	Deprecated       bool
	DeprecatedReason string
	IsForm           bool
	Struct           *Struct
	Union            *Union
	Enum             *Enum
	Target           Type
	Validation       *Validation
	Error            *ErrorMessage
	Masks            []*Mask
	Origin           diag.Origin

	schema    *spec.Schema
	enumNames []string
}

// Validation is what the Validate methods of a declaration check. Count is the runtime check of
// how many union variants are set, empty for none. HasResponse adds ValidateResponse, whose checks
// differ from those of Validate.
type Validation struct {
	Count           string
	IsDiscriminated bool
	Checks          []*Check
	HasResponse     bool
}

// Check is what Validate checks of one value. Field is the Go field it lives in, empty for the value
// itself, and Path its JSON name in error paths. IsGuarded skips the checks of a nil value, which is
// absent; IsRequired reports it. Nested is the declaration whose Validate is called, nil with
// IsNested for runtime.Email.
type Check struct {
	Field      string
	Path       string
	IsPointer  bool
	IsGuarded  bool
	IsRequired bool
	Side       Side
	Rules      []Rule
	IsNested   bool
	Nested     *Decl
	Items      *Check
	Values     *Check
	Keys       []Rule
}

// Rule is one keyword check. Number is a bound, length, count or factor as the spec writes it.
type Rule struct {
	Kind        RuleKind
	Number      string
	IsExclusive bool
	IsBase64    bool
	Format      string
	Pattern     *Pattern
	Const       spec.Value
	Values      []spec.Value
}

// Pattern is the Text of a spec pattern, compiled once from its Go Source into a variable of Part.
type Pattern struct {
	Name   string
	Text   string
	Source string
	Part   string
	Origin diag.Origin
}

// MaskKind is what Masked does to one value.
type MaskKind int

const (
	MaskFull MaskKind = iota
	MaskRegex
	MaskHash
	MaskPartial
	// MaskZero clears a sensitive value that is no string.
	MaskZero
	// MaskNested calls Masked of the value's own type.
	MaskNested
	// MaskItems masks each item of a slice.
	MaskItems
	// MaskValues masks each value of a map.
	MaskValues
)

// Mask is one change Masked makes to a copy. Field is the Go field, empty for the value itself.
// Pattern is the expression of MaskRegex; KeepPrefix and KeepSuffix the characters MaskPartial
// leaves visible.
type Mask struct {
	Field      string
	Kind       MaskKind
	IsPointer  bool
	Pattern    *Pattern
	KeepPrefix int
	KeepSuffix int
}

// ErrorMessage is where an error type keeps its message. HasConstructor adds a NewT function;
// Unset lists, by JSON path, the required properties it leaves empty.
type ErrorMessage struct {
	Path           string
	HasConstructor bool
	Unset          []string
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
// IsText is a union of scalars with no shared properties, which a parameter writes as text.
type Union struct {
	IsAnyOf       bool
	IsNullable    bool
	IsText        bool
	Discriminator string
	Variants      []*Variant

	isTypeList bool
}

// Variant is one member of a union. FieldType is a pointer to Type unless Type can be nil. The
// other fields tell decoding which JSON picks it; Known is nil when any key fits. Shapes are the
// objects a variant that is itself a union can be, at any depth.
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
	Shapes    []Shape
	Origin    diag.Origin

	schema *spec.Schema
}

// Shape is one object a union variant can be.
type Shape struct {
	Required []string
	Known    []string
	IsClosed bool
}

// Field is one struct field. Required, Nullable, ReadOnly and WriteOnly repeat the spec.
// IsJSONIgnored writes the field with the JSON tag "-". Sensitive is how Masked masks it, nil for
// a value that is not sensitive. Value and Default are what the server checks and sets in a body.
// Getter returns the field or its default, nil when the field has none.
type Field struct {
	Name             string
	JSONName         string
	Type             Type
	Required         bool
	Nullable         bool
	OmitEmpty        bool
	ReadOnly         bool
	WriteOnly        bool
	Deprecated       bool
	DeprecatedReason string
	IsJSONIgnored    bool
	Doc              string
	Tags             []Tag
	Sensitive        *extension.Mask
	Value            *BodyValue
	Default          string
	Getter           *Getter
	Origin           diag.Origin

	schema           *spec.Schema
	def              *spec.Value
	goName           string
	isPointerSkipped bool
}

// Tag is a struct tag written next to json, such as yaml:"name,omitempty".
type Tag struct {
	Key   string
	Value string
}

// Enum is a named type over Base with one constant per value, in spec order.
type Enum struct {
	Base       Type
	Values     []EnumValue
	ValuesFunc string
}

// Const returns the name of the constant that holds v.
func (e *Enum) Const(v spec.Value) (string, bool) {
	i := slices.IndexFunc(e.Values, func(ev EnumValue) bool { return sameValue(ev.Value, v) })
	if i < 0 {
		return "", false
	}
	return e.Values[i].Name, true
}

type EnumValue struct {
	Name  string
	Value spec.Value
}

func origin(o spec.Origin) diag.Origin {
	return diag.Origin{File: o.File, Line: o.Line, Col: o.Col}
}
