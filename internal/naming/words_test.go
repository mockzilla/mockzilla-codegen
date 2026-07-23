// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package naming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRawWords(t *testing.T) {
	t.Parallel()

	initialisms := map[string]string{"id": "ID", "http": "HTTP"}
	tests := []struct {
		name string
		part string
		want []string
	}{
		{name: "Camel case", part: "fooBarBaz", want: []string{"foo", "Bar", "Baz"}},
		{name: "Upper run gives its last capital to the next word", part: "HTTPServer", want: []string{"HTTP", "Server"}},
		{name: "Upper run keeps a plural s when it is an initialism", part: "userIDs", want: []string{"user", "IDs"}},
		{name: "Plural s followed by a lower-case letter splits as usual", part: "IDsa", want: []string{"I", "Dsa"}},
		{name: "Plural s before a capital", part: "IDsLeft", want: []string{"IDs", "Left"}},
		{name: "Letters after digits that follow letters", part: "v2api", want: []string{"v2", "api"}},
		{name: "Letters after leading digits stay", part: "1st", want: []string{"1st"}},
		{name: "Capitals after digits", part: "404NotFound", want: []string{"404", "Not", "Found"}},
		{name: "Separators", part: "a_b-c", want: []string{"a", "b", "c"}},
		{name: "Meaningful symbols become words", part: "-a+b@c", want: []string{"Minus", "a", "Plus", "b", "At", "c"}},
		{name: "Decimal point", part: "1.5", want: []string{"1", "Dot5"}},
		{name: "Diacritics fold", part: "Ñandú", want: []string{"Nandu"}},
		{name: "Symbols only", part: "<=", want: []string{"Less", "Than", "Equal"}},
		{name: "Hex fallback", part: "名", want: []string{"X540", "D"}},
		{name: "Empty", part: "", want: []string{"Empty"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, rawWords(tt.part, initialisms))
		})
	}
}
