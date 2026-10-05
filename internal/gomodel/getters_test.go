// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestPlanGetters(t *testing.T) {
	t.Parallel()

	checkGolden(t, "getters", "getters", testOptions())
}

func TestIsConstant(t *testing.T) {
	t.Parallel()

	number := func(n string) spec.Value { return spec.Value{Kind: spec.KindNumber, Num: json.Number(n)} }
	tests := []struct {
		name   string
		typ    string
		value  spec.Value
		isTrue bool
	}{
		{name: "String", typ: "string", value: spec.Value{Kind: spec.KindString, Str: "a"}, isTrue: true},
		{name: "Bool", typ: "bool", value: spec.Value{Kind: spec.KindBool}, isTrue: true},
		{name: "Number for a string", typ: "string", value: number("1")},
		{name: "String for a number", typ: "int", value: spec.Value{Kind: spec.KindString, Str: "1"}},
		{name: "Float64", typ: "float64", value: number("1e300"), isTrue: true},
		{name: "Float32 out of range", typ: "float32", value: number("1e39")},
		{name: "Fraction for an integer", typ: "int", value: number("1.5")},
		{name: "Integral fraction for an integer", typ: "int", value: number("2.0"), isTrue: true},
		{name: "Lowest int8", typ: "int8", value: number("-128"), isTrue: true},
		{name: "Below int8", typ: "int8", value: number("-129")},
		{name: "Above int8", typ: "int8", value: number("128")},
		{name: "Highest uint64", typ: "uint64", value: number("18446744073709551615"), isTrue: true},
		{name: "Negative uint", typ: "uint", value: number("-1")},
		{name: "No number type", typ: "any", value: number("1")},
		{name: "No number", typ: "int", value: number("one")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.isTrue, isConstant(tc.typ, tc.value))
		})
	}
}
