// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmailValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		email Email
		want  error
	}{
		{name: "Bare address", email: "a@example.com"},
		{name: "Empty", email: "", want: ErrInvalidEmail},
		{name: "No at sign", email: "example.com", want: ErrInvalidEmail},
		{name: "Display name", email: "A <a@example.com>", want: ErrInvalidEmail},
		{name: "Angle brackets", email: "<a@example.com>", want: ErrInvalidEmail},
		{name: "Leading space", email: " a@example.com", want: ErrInvalidEmail},
		{name: "Space after the at sign", email: "a@ example.com", want: ErrInvalidEmail},
		{name: "Dots and tags", email: "first.last+tag@example.com"},
		{name: "Two dots in a row", email: "a..b@example.com", want: ErrInvalidEmail},
		{name: "Leading dot", email: ".a@example.com", want: ErrInvalidEmail},
		{name: "Empty local part", email: "@example.com", want: ErrInvalidEmail},
		{name: "Quoted local part", email: `"a b"@example.com`},
		{name: "Quotes without need", email: `"a"@example.com`},
		{name: "Empty quotes", email: `""@example.com`},
		{name: "At sign in quotes", email: `"a@b"@example.com`},
		{name: "Escaped quote", email: `"a\"b"@example.com`},
		{name: "Quote not closed", email: `"a@example.com`, want: ErrInvalidEmail},
		{name: "Quote inside quotes", email: `"a"b"@example.com`, want: ErrInvalidEmail},
		{name: "Escape at the end", email: `"a\"@example.com`, want: ErrInvalidEmail},
		{name: "Tab in quotes", email: "\"a\tb\"@example.com", want: ErrInvalidEmail},
		{name: "Character outside ASCII", email: "josé@example.com", want: ErrInvalidEmail},
		{name: "Host name of one label", email: "a@localhost"},
		{name: "Underscore in the domain", email: "a@b_c", want: ErrInvalidEmail},
		{name: "Dot at the end", email: "a@example.com.", want: ErrInvalidEmail},
		{name: "IPv4 literal", email: "a@[10.0.0.1]"},
		{name: "IPv6 literal", email: "a@[IPv6:::1]"},
		{name: "IPv6 tag in lower case", email: "a@[ipv6:::1]"},
		{name: "IPv6 without its tag", email: "a@[::1]", want: ErrInvalidEmail},
		{name: "IPv6 tag alone", email: "a@[IPv6:]", want: ErrInvalidEmail},
		{name: "Bracket not closed", email: "a@[10.0.0.1", want: ErrInvalidEmail},
		{name: "Longest local part", email: Email(strings.Repeat("a", 64) + "@example.com")},
		{name: "Local part too long", email: Email(strings.Repeat("a", 65) + "@example.com"), want: ErrInvalidEmail},
		{name: "Address too long", email: Email("a@" + strings.Repeat("a.", 126) + "a"), want: ErrInvalidEmail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.email.Validate())
		})
	}
}
