// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"encoding/json"
	"testing"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/spec"
)

func TestBoundSourceResolve(t *testing.T) {
	t.Parallel()

	flag := func(b bool) *base.DynamicValue[bool, float64] { return &base.DynamicValue[bool, float64]{N: 0, A: b} }
	num := func(f float64) *base.DynamicValue[bool, float64] {
		return &base.DynamicValue[bool, float64]{N: 1, B: f}
	}
	tests := []struct {
		name    string
		src     boundSource
		isLower bool
		want    *spec.Bound
	}{
		{name: "Nothing set", isLower: true},
		{name: "Inclusive only", src: boundSource{value: new(3.0)}, isLower: true, want: &spec.Bound{Value: "3"}},
		{name: "3.0 flag makes the value exclusive", src: boundSource{value: new(3.0), exclusive: flag(true)}, isLower: true, want: &spec.Bound{Value: "3", Exclusive: true}},
		{name: "3.0 false flag keeps it inclusive", src: boundSource{value: new(3.0), exclusive: flag(false)}, isLower: true, want: &spec.Bound{Value: "3"}},
		{name: "3.0 flag without a value means nothing", src: boundSource{exclusive: flag(true)}, isLower: true},
		{name: "3.1 exclusive only", src: boundSource{exclusive: num(5)}, isLower: true, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Lower: larger exclusive wins", src: boundSource{value: new(3.0), exclusive: num(5)}, isLower: true, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Lower: larger inclusive wins", src: boundSource{value: new(7.0), exclusive: num(5)}, isLower: true, want: &spec.Bound{Value: "7"}},
		{name: "Lower: equal values are exclusive", src: boundSource{value: new(5.0), exclusive: num(5)}, isLower: true, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Upper: smaller exclusive wins", src: boundSource{value: new(10.0), exclusive: num(5)}, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Upper: smaller inclusive wins", src: boundSource{value: new(3.0), exclusive: num(5)}, want: &spec.Bound{Value: "3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.src.resolve(tt.isLower))
		})
	}
}

func TestNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node *yaml.Node
		f    float64
		want json.Number
	}{
		{name: "Written form is kept", node: &yaml.Node{Value: "1.50"}, f: 1.5, want: "1.50"},
		{name: "No node formats the float", f: 0.25, want: "0.25"},
		{name: "YAML-only form is reformatted", node: &yaml.Node{Value: "+2"}, f: 2, want: "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, number(tt.node, tt.f))
		})
	}
}
