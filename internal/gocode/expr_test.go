// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpressions(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "p.Name", Selector("p", "Name"))
	assert.Equal(t, "*p.Age", Deref(Selector("p", "Age")))
	assert.Equal(t, "runtime.MinLength(v, 1)", Call("runtime.MinLength", "v", "1"))
	assert.Equal(t, "p.Validate()", Call("p.Validate"))
	assert.Equal(t, "m[key]", Index("m", "key"))
	assert.Equal(t, "p.Cat != nil", NotNil("p.Cat"))
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
