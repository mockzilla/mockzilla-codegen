// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

var (
	importTime    = Import{Path: "time"}
	importJSON    = Import{Path: "encoding/json"}
	importRuntime = Import{Path: RuntimePath}

	anyType    = Builtin{Name: "any"}
	stringType = Builtin{Name: "string"}
	boolType   = Builtin{Name: "bool"}
	rawJSON    = Qualified{Import: importJSON, Name: "RawMessage"}
)

// Read-only tables. Formats are looked up in lower case.
var (
	stringFormats = map[string]Type{
		"date":      Qualified{Import: importRuntime, Name: "Date"},
		"date-time": Qualified{Import: importTime, Name: "Time"},
		"email":     Qualified{Import: importRuntime, Name: "Email"},
		"byte":      Slice{Elem: Builtin{Name: "byte"}},
		"binary":    Qualified{Import: importRuntime, Name: "File"},
		"json":      rawJSON,
	}
	integerFormats = map[string]string{
		"int8": "int8", "int16": "int16", "int32": "int32", "int64": "int64",
		"uint": "uint", "uint8": "uint8", "uint16": "uint16", "uint32": "uint32", "uint64": "uint64",
	}
	numberFormats = map[string]string{
		"float": "float32", "double": "float64", "int32": "int32", "int64": "int64",
	}
)

// primitive maps a schema with at most one type to a Go type. docs/types.md has the table.
func primitive(s *spec.Schema, intType string) Type {
	format := strings.ToLower(s.Format)
	switch s.Types &^ spec.TypeNull {
	case spec.TypeString:
		if t, ok := stringFormats[format]; ok {
			return t
		}
		return stringType
	case spec.TypeInteger:
		return integerType(format, intType)
	case spec.TypeNumber:
		return numberType(format, intType)
	case spec.TypeBoolean:
		return boolType
	default:
		return untyped(s, format, intType)
	}
}

func integerType(format, intType string) Type {
	if name, ok := integerFormats[format]; ok {
		return Builtin{Name: name}
	}
	return Builtin{Name: intType}
}

// numberType also reads the integer formats some specs put on numbers.
func numberType(format, intType string) Type {
	if name, ok := numberFormats[format]; ok {
		return Builtin{Name: name}
	}
	if format == "integer" || format == "int" {
		return Builtin{Name: intType}
	}
	return Builtin{Name: "float64"}
}

// untyped guesses a schema without a type from its format, then from its const.
func untyped(s *spec.Schema, format, intType string) Type {
	if t, ok := stringFormats[format]; ok {
		return t
	}
	if name, ok := integerFormats[format]; ok {
		return Builtin{Name: name}
	}
	if name, ok := numberFormats[format]; ok {
		return Builtin{Name: name}
	}
	if s.Const != nil {
		return constType(*s.Const, intType)
	}
	return anyType
}

func constType(v spec.Value, intType string) Type {
	switch v.Kind {
	case spec.KindString:
		return stringType
	case spec.KindNumber:
		if isIntegral(v.Num) {
			return Builtin{Name: intType}
		}
		return Builtin{Name: "float64"}
	case spec.KindBool:
		return boolType
	default:
		return anyType
	}
}
