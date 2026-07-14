// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package spec

import "encoding/json"

// TypeSet holds the allowed JSON types; Null stays only when alone, else it becomes Nullable.
type TypeSet uint8

const (
	TypeString TypeSet = 1 << iota
	TypeNumber
	TypeInteger
	TypeBoolean
	TypeObject
	TypeArray
	TypeNull
)

func (t TypeSet) Has(other TypeSet) bool {
	return t&other == other && other != 0
}

type AdditionalMode int

const (
	AdditionalUnset AdditionalMode = iota
	AdditionalAllowed
	AdditionalDenied
	AdditionalSchema
)

// Schema is one schema; on a $ref usage Ref is set and keywords next to the $ref stay here.
type Schema struct {
	Ref                  *Ref
	Types                TypeSet
	Nullable             bool
	Format               string
	Title                string
	Description          string
	Pattern              string
	ContentEncoding      string
	ContentMediaType     string
	Required             []string
	Properties           []*Property
	AdditionalProperties Additional
	Items                *Schema
	PrefixItems          []*Schema
	AllOf                []*Schema
	OneOf                []*Schema
	AnyOf                []*Schema
	Not                  *Schema
	If                   *Schema
	Then                 *Schema
	Else                 *Schema
	Discriminator        *Discriminator
	Enum                 []Value
	Const                *Value
	Default              *Value
	Examples             []Value
	Limits               Limits
	ReadOnly             bool
	WriteOnly            bool
	Deprecated           bool
	Extensions           []Extension
	Origin               Origin
}

// Ref is a schema $ref; Name is set for a component target, and Target can close a cycle.
type Ref struct {
	Pointer string
	Name    string
	Target  *Schema
}

type Property struct {
	Name     string
	Schema   *Schema
	Required bool
}

type Additional struct {
	Mode   AdditionalMode
	Schema *Schema
}

// Discriminator has every mapping value resolved, bare names too; Default is 3.2 defaultMapping.
type Discriminator struct {
	Property string
	Mapping  []Mapping
	Default  *Ref
}

type Mapping struct {
	Value string
	Ref   *Ref
}

// Limits keeps numbers as written; 3.0 boolean and 3.1 numeric exclusive bounds look the same.
type Limits struct {
	Minimum       *Bound
	Maximum       *Bound
	MultipleOf    *json.Number
	MinLength     *int64
	MaxLength     *int64
	MinItems      *int64
	MaxItems      *int64
	MinProperties *int64
	MaxProperties *int64
	UniqueItems   bool
}

type Bound struct {
	Value     json.Number
	Exclusive bool
}
