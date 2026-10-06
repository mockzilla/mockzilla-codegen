// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

func TestExpressions(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "p.Name", Selector("p", "Name"))
	assert.Equal(t, "*p.Age", Deref(Selector("p", "Age")))
	assert.Equal(t, "runtime.MinLength(v, 1)", Call("runtime.MinLength", "v", "1"))
	assert.Equal(t, "p.Validate()", Call("p.Validate"))
	assert.Equal(t, "m[key]", Index("m", "key"))
	assert.Equal(t, "p.Cat != nil", NotNil("p.Cat"))
	assert.Equal(t, "p == nil", IsNil("p"))
	assert.Equal(t, "p == nil || p.Cat == nil", Or(IsNil("p"), IsNil("p.Cat")))
	assert.Equal(t, "value, ok := p.Cat.Get(); ok", Get("p.Cat", "value", "ok"))
	assert.Equal(t, `opts.Body != ""`, NotEmpty("opts.Body"))
	assert.Equal(t, "&opts.Body", AddressOf("opts.Body"))
	assert.Equal(t, "(w http.ResponseWriter, r *http.Request)", Signature([]string{Param("w", "http.ResponseWriter"), Param("r", Deref("http.Request"))}, ""))
	assert.Equal(t, "(c echo.Context) error", Signature([]string{Param("c", "echo.Context")}, "error"))
	assert.Equal(t, "w, r := c.Response(), c.Request()", Define([]string{"w", "r"}, Call("c.Response"), Call("c.Request")))
	assert.Equal(t, "return", Return())
	assert.Equal(t, "return nil", Return("nil"))
	assert.Equal(t, `T{"a": 1}`, Composite("T", []KeyValue{{Key: `"a"`, Value: "1"}}))
	assert.Equal(t, "T{\n\"a\": 1,\n\"b\": 2,\n}", Composite("T", []KeyValue{{Key: `"a"`, Value: "1"}, {Key: `"b"`, Value: "2"}}))
	assert.Equal(t, `""`, Zero(gomodel.Builtin{Name: "string"}))
	assert.Equal(t, "nil", Zero(gomodel.Pointer{Elem: gomodel.Builtin{Name: "string"}}))
}

func TestDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "Whole seconds", d: 30 * time.Second, want: "30 * time.Second"},
		{name: "Whole hours", d: 2 * time.Hour, want: "2 * time.Hour"},
		{name: "Milliseconds", d: 1500 * time.Millisecond, want: "1500 * time.Millisecond"},
		{name: "Nanoseconds", d: 1001, want: "1001 * time.Nanosecond"},
		{name: "Zero", d: 0, want: "0 * time.Hour"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Duration(tc.d, "time"))
		})
	}
}

func TestRawString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want string
	}{
		{name: "Backslashes stay as written", s: `^\d+$`, want: "`^\\d+$`"},
		{name: "Backquote", s: "a`b", want: `"a` + "`" + `b"`},
		{name: "Carriage return", s: "a\rb", want: `"a\rb"`},
		{name: "Control character", s: "a\x00b", want: `"a\x00b"`},
		{name: "Non-ASCII letters", s: "^é", want: "`^é`"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, RawString(tc.s))
		})
	}
}
