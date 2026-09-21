// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package jsonschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectMarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		obj  *Object
		want string
	}{
		{name: "Empty", obj: &Object{}, want: `{}`},
		{name: "Keys keep their order", obj: new(Object).Set("b", "1").Set("a", "2"), want: `{"b":"1","a":"2"}`},
		{name: "A key set again keeps its place", obj: new(Object).Set("b", "1").Set("a", "2").Set("b", "3"), want: `{"b":"3","a":"2"}`},
		{name: "Every value kind", obj: new(Object).Set("n", nil).Set("t", true).Set("f", false).Set("num", json.Number("1.50")).Set("list", []string{"x", "y"}).Set("any", []any{json.Number("1"), "s", &Object{}}).Set("obj", new(Object).Set("k", "v")), want: `{"n":null,"t":true,"f":false,"num":1.50,"list":["x","y"],"any":[1,"s",{}],"obj":{"k":"v"}}`},
		{name: "HTML characters stay as they are", obj: new(Object).Set("a<b", "x & y"), want: `{"a<b":"x & y"}`},
		{name: "A number that is no JSON is quoted", obj: new(Object).Set("n", json.Number("1e")), want: `{"n":"1e"}`},
		{name: "A value of another kind is null", obj: new(Object).Set("n", 42), want: `{"n":null}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.obj.MarshalJSON()

			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

func TestObjectLen(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, new(Object).Len())
	assert.Equal(t, 1, new(Object).Set("a", "1").Set("a", "2").Len())
}
