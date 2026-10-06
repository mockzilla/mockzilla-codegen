// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		format string
		value  string
		want   bool
	}{
		{name: "UUID", format: "uuid", value: "0F8FAD5B-D9CB-469F-A165-70867728950E", want: true},
		{name: "UUID too short", format: "uuid", value: "0f8fad5b"},
		{name: "UUID without dashes", format: "uuid", value: "0f8fad5bxd9cb-469f-a165-70867728950e"},
		{name: "UUID with a letter out of hex", format: "uuid", value: "0f8fad5b-d9cb-469f-a165-70867728950g"},
		{name: "URI", format: "uri", value: "https://example.com/a", want: true},
		{name: "URI without scheme", format: "uri", value: "/a"},
		{name: "URI reference", format: "uri-reference", value: "/a?b=c", want: true},
		{name: "Broken URI reference", format: "uri-reference", value: "%zz"},
		{name: "IPv4", format: "ipv4", value: "10.0.0.1", want: true},
		{name: "IPv6 is no IPv4", format: "ipv4", value: "::1"},
		{name: "IPv6", format: "ipv6", value: "fe80::1", want: true},
		{name: "IPv6 with a zone", format: "ipv6", value: "fe80::1%eth0"},
		{name: "IPv4 is no IPv6", format: "ipv6", value: "10.0.0.1"},
		{name: "Host name", format: "hostname", value: "api.example-1.com", want: true},
		{name: "Empty host name", format: "hostname"},
		{name: "Host name too long", format: "hostname", value: strings.Repeat("a.", 127) + "a"},
		{name: "Empty label", format: "hostname", value: "a..b"},
		{name: "Label starting with a dash", format: "hostname", value: "-a.b"},
		{name: "Label with an underscore", format: "hostname", value: "a_b.c"},
		{name: "Email", format: "email", value: "a@example.com", want: true},
		{name: "Email with a name", format: "email", value: "A <a@example.com>"},
		{name: "Date", format: "date", value: "2026-09-30", want: true},
		{name: "Bad date", format: "date", value: "2026-13-01"},
		{name: "Date-time", format: "date-time", value: "2026-09-30T10:00:00+02:00", want: true},
		{name: "Date-time without zone", format: "date-time", value: "2026-09-30T10:00:00"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, Format(tc.value, tc.format) == nil)
		})
	}
}

func TestIsEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		email     string
		isInvalid bool
	}{
		{name: "Bare address", email: "a@example.com"},
		{name: "Empty", email: "", isInvalid: true},
		{name: "No at sign", email: "example.com", isInvalid: true},
		{name: "Display name", email: "A <a@example.com>", isInvalid: true},
		{name: "Angle brackets", email: "<a@example.com>", isInvalid: true},
		{name: "Leading space", email: " a@example.com", isInvalid: true},
		{name: "Space after the at sign", email: "a@ example.com", isInvalid: true},
		{name: "Dots and tags", email: "first.last+tag@example.com"},
		{name: "Two dots in a row", email: "a..b@example.com", isInvalid: true},
		{name: "Leading dot", email: ".a@example.com", isInvalid: true},
		{name: "Empty local part", email: "@example.com", isInvalid: true},
		{name: "Quoted local part", email: `"a b"@example.com`},
		{name: "Quotes without need", email: `"a"@example.com`},
		{name: "Empty quotes", email: `""@example.com`},
		{name: "At sign in quotes", email: `"a@b"@example.com`},
		{name: "Escaped quote", email: `"a\"b"@example.com`},
		{name: "Quote not closed", email: `"a@example.com`, isInvalid: true},
		{name: "Quote inside quotes", email: `"a"b"@example.com`, isInvalid: true},
		{name: "Escape at the end", email: `"a\"@example.com`, isInvalid: true},
		{name: "Tab in quotes", email: "\"a\tb\"@example.com", isInvalid: true},
		{name: "Character outside ASCII", email: "josé@example.com", isInvalid: true},
		{name: "Host name of one label", email: "a@localhost"},
		{name: "Underscore in the domain", email: "a@b_c", isInvalid: true},
		{name: "Dot at the end", email: "a@example.com.", isInvalid: true},
		{name: "IPv4 literal", email: "a@[10.0.0.1]"},
		{name: "IPv6 literal", email: "a@[IPv6:::1]"},
		{name: "IPv6 tag in lower case", email: "a@[ipv6:::1]"},
		{name: "IPv6 without its tag", email: "a@[::1]", isInvalid: true},
		{name: "IPv6 tag alone", email: "a@[IPv6:]", isInvalid: true},
		{name: "Bracket not closed", email: "a@[10.0.0.1", isInvalid: true},
		{name: "Longest local part", email: strings.Repeat("a", 64) + "@example.com"},
		{name: "Local part too long", email: strings.Repeat("a", 65) + "@example.com", isInvalid: true},
		{name: "Address too long", email: "a@" + strings.Repeat("a.", 126) + "a", isInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, !tt.isInvalid, isEmail(tt.email))
		})
	}
}
