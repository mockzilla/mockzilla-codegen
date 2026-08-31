// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssign(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dst     any
		value   any
		want    any
		wantErr bool
	}{
		{name: "Unsigned", dst: new(uint16), value: "7", want: uint16(7)},
		{name: "Float", dst: new(float32), value: "1.5", want: float32(1.5)},
		{name: "Boolean", dst: new(bool), value: "true", want: true},
		{name: "Bad boolean", dst: new(bool), value: "yes", wantErr: true},
		{name: "Text into a slice", dst: new([]string), value: "a", want: []string{"a"}},
		{name: "JSON text into a struct", dst: new(address), value: `{"city":"x"}`, want: address{City: "x"}},
		{name: "JSON text into a map", dst: new(map[string]int), value: `{"a":1}`, want: map[string]int{"a": 1}},
		{name: "Text into a channel", dst: new(chan int), value: "a", wantErr: true},
		{name: "List with a bad item", dst: new([]int), value: []string{"1", "x"}, wantErr: true},
		{name: "Items into an untyped target", dst: new(any), value: []any{"1", map[string]any{"a": "b"}}, want: []any{"1", map[string]any{"a": "b"}}},
		{name: "Empty list into a value", dst: new(string), value: []string{}, want: ""},
		{name: "First item into a value", dst: new(int), value: []string{"3", "4"}, want: 3},
		{name: "Object with a bad field", dst: new(rgb), value: map[string]string{"R": "x"}, wantErr: true},
		{name: "Object with a bad map value", dst: new(map[string]int), value: map[string]string{"a": "x"}, wantErr: true},
		{name: "Object into an untyped target", dst: new(any), value: map[string]any{"a": []string{"1"}}, want: map[string]any{"a": []any{"1"}}},
		{name: "Object into a value", dst: new(int), value: map[string]string{"a": "1"}, wantErr: true},
		{name: "Value of another kind is left alone", dst: new(int), value: 5, want: 0},
		{name: "Flat object into an untyped target", dst: new(any), value: map[string]string{"a": "1"}, want: map[string]any{"a": "1"}},
		{name: "Number into an untyped target", dst: new(any), value: 5, want: 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := assigner{}.assign(reflect.ValueOf(tc.dst).Elem(), tc.value)

			if tc.wantErr {
				require.ErrorIs(t, err, ErrParamValue)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, reflect.ValueOf(tc.dst).Elem().Interface())
		})
	}
}
