// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"encoding/json"
	"math/big"
	"strconv"

	"github.com/mockzilla/codegen/internal/spec"
)

// enumKind is the type of an enum's constants.
type enumKind int

const (
	enumNone enumKind = iota
	enumString
	enumInteger
	enumNumber
	enumBool
)

// enumKindOf takes the type keyword, else the kind all values share. Objects and arrays have none.
func enumKindOf(s *spec.Schema) enumKind {
	switch s.Types &^ spec.TypeNull {
	case spec.TypeString:
		return enumString
	case spec.TypeInteger:
		return enumInteger
	case spec.TypeNumber:
		return enumNumber
	case spec.TypeBoolean:
		return enumBool
	case 0:
		return inferKind(s.Enum)
	default:
		return enumNone
	}
}

// inferKind lets integers and other numbers mix; any other mix has no kind.
func inferKind(values []spec.Value) enumKind {
	kind := enumNone
	for _, v := range values {
		k := valueKind(v)
		switch {
		case v.Kind == spec.KindNull:
		case k == enumNone:
			return enumNone
		case kind == enumNone, kind == k:
			kind = k
		case kind == enumInteger && k == enumNumber, kind == enumNumber && k == enumInteger:
			kind = enumNumber
		default:
			return enumNone
		}
	}
	return kind
}

func valueKind(v spec.Value) enumKind {
	switch v.Kind {
	case spec.KindString:
		return enumString
	case spec.KindNumber:
		if isIntegral(v.Num) {
			return enumInteger
		}
		return enumNumber
	case spec.KindBool:
		return enumBool
	default:
		return enumNone
	}
}

// enumValues keeps the values that fit kind, once each, in spec order. Null is left out: it makes
// the type nullable instead.
func enumValues(kind enumKind, values []spec.Value) []spec.Value {
	var out []spec.Value
	seen := map[string]bool{}
	for _, v := range values {
		fv, ok := fit(kind, v)
		if !ok || seen[valueText(fv)] {
			continue
		}
		seen[valueText(fv)] = true
		out = append(out, fv)
	}
	return out
}

// misfits lists the values an enum of kind leaves out, null apart.
func misfits(kind enumKind, values []spec.Value) []spec.Value {
	var out []spec.Value
	for _, v := range values {
		if _, ok := fit(kind, v); !ok && v.Kind != spec.KindNull {
			out = append(out, v)
		}
	}
	return out
}

// fit returns v as a constant of kind. Numbers and booleans in a string enum become strings.
func fit(kind enumKind, v spec.Value) (spec.Value, bool) {
	switch {
	case kind == enumString && v.Kind == spec.KindString:
		return v, true
	case kind == enumString && (v.Kind == spec.KindNumber || v.Kind == spec.KindBool):
		return spec.Value{Kind: spec.KindString, Str: valueText(v)}, true
	case kind == enumInteger && v.Kind == spec.KindNumber && isIntegral(v.Num),
		kind == enumNumber && v.Kind == spec.KindNumber,
		kind == enumBool && v.Kind == spec.KindBool:
		return v, true
	}
	return spec.Value{}, false
}

func enumBase(kind enumKind, format, intType string) Type {
	switch kind {
	case enumInteger:
		return integerType(format, intType)
	case enumNumber:
		return numberType(format, intType)
	case enumBool:
		return boolType
	default:
		return stringType
	}
}

// valueText is the text a scalar is named from and compared by.
func valueText(v spec.Value) string {
	switch v.Kind {
	case spec.KindNumber:
		return v.Num.String()
	case spec.KindBool:
		return strconv.FormatBool(v.Bool)
	default:
		return v.Str
	}
}

func isIntegral(n json.Number) bool {
	f, ok := new(big.Float).SetString(n.String())
	return ok && f.IsInt()
}
