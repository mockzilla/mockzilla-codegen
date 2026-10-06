// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package validation

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		check func(set ...bool) error
		set   []bool
		want  error
	}{
		{name: "Exactly one with one", check: ExactlyOne, set: []bool{false, true}},
		{name: "Exactly one with none", check: ExactlyOne, set: []bool{false, false}, want: Error{Message: "exactly one variant must be set, found 0", Rule: RuleOneOf}},
		{name: "Exactly one with two", check: ExactlyOne, set: []bool{true, true}, want: Error{Message: "exactly one variant must be set, found 2", Rule: RuleOneOf}},
		{name: "At most one with none", check: AtMostOne, set: []bool{false, false}},
		{name: "At most one with two", check: AtMostOne, set: []bool{true, true}, want: Error{Message: "at most one variant may be set, found 2", Rule: RuleOneOf}},
		{name: "At least one with two", check: AtLeastOne, set: []bool{true, true}},
		{name: "At least one with none", check: AtLeastOne, set: []bool{false}, want: Error{Message: "at least one variant must be set", Rule: RuleAnyOf}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.check(tc.set...))
		})
	}
}

func TestAnyValid(t *testing.T) {
	t.Parallel()

	short := Errors{{Message: "must be at most 5 characters long", Rule: RuleMaxLength}}
	early := Errors{{Message: "must be at least 2026", Rule: RuleMinimum}}
	both := slices.Concat(short, early)
	tests := []struct {
		name     string
		variants []VariantErrors
		want     error
	}{
		{name: "Nothing set", variants: []VariantErrors{{Errs: short}}},
		{name: "A set variant that passes", variants: []VariantErrors{{IsSet: true}, {IsSet: true, Errs: short}}},
		{name: "A passing variant later", variants: []VariantErrors{{IsSet: true, Errs: short}, {IsSet: true}}},
		{name: "Every set variant fails", variants: []VariantErrors{{IsSet: true, Errs: short}, {Errs: early}, {IsSet: true, Errs: early}}, want: &both},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, AnyValid(tc.variants...))
		})
	}
}
