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

func TestEnumConst(t *testing.T) {
	t.Parallel()

	e := &Enum{Base: Builtin{Name: "int"}, Values: []EnumValue{
		{Name: "LevelLow", Value: spec.Value{Kind: spec.KindNumber, Num: json.Number("1")}},
		{Name: "LevelHigh", Value: spec.Value{Kind: spec.KindNumber, Num: json.Number("2")}},
	}}
	tests := []struct {
		name    string
		value   spec.Value
		want    string
		isFound bool
	}{
		{name: "Same number in another form", value: spec.Value{Kind: spec.KindNumber, Num: json.Number("2.0")}, want: "LevelHigh", isFound: true},
		{name: "Value of no constant", value: spec.Value{Kind: spec.KindNumber, Num: json.Number("3")}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			name, isFound := e.Const(tc.value)
			assert.Equal(t, tc.want, name)
			assert.Equal(t, tc.isFound, isFound)
		})
	}
}
