// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestBoundSourceResolve(t *testing.T) {
	t.Parallel()

	flag := func(b bool) *spec.Value { return &spec.Value{Kind: spec.KindBool, Bool: b} }
	num := func(n json.Number) *spec.Value { return &spec.Value{Kind: spec.KindNumber, Num: n} }
	tests := []struct {
		name    string
		src     boundSource
		isLower bool
		want    *spec.Bound
	}{
		{name: "Nothing set", isLower: true},
		{name: "Inclusive only", src: boundSource{value: num("3")}, isLower: true, want: &spec.Bound{Value: "3"}},
		{name: "Flag makes the value exclusive", src: boundSource{value: num("3"), exclusive: flag(true)}, isLower: true, want: &spec.Bound{Value: "3", Exclusive: true}},
		{name: "False flag keeps it inclusive", src: boundSource{value: num("3"), exclusive: flag(false)}, isLower: true, want: &spec.Bound{Value: "3"}},
		{name: "Flag without a value means nothing", src: boundSource{exclusive: flag(true)}, isLower: true},
		{name: "Exclusive number only", src: boundSource{exclusive: num("5")}, isLower: true, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Lower: larger exclusive wins", src: boundSource{value: num("3"), exclusive: num("5")}, isLower: true, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Lower: larger inclusive wins", src: boundSource{value: num("7"), exclusive: num("5")}, isLower: true, want: &spec.Bound{Value: "7"}},
		{name: "Lower: equal values are exclusive", src: boundSource{value: num("5.0"), exclusive: num("5")}, isLower: true, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Upper: smaller exclusive wins", src: boundSource{value: num("10"), exclusive: num("5")}, want: &spec.Bound{Value: "5", Exclusive: true}},
		{name: "Upper: smaller inclusive wins", src: boundSource{value: num("3"), exclusive: num("5")}, want: &spec.Bound{Value: "3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.src.resolve(tt.isLower))
		})
	}
}
