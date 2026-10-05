// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
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
		{name: "Exactly one with none", check: ExactlyOne, set: []bool{false, false}, want: ValidationError{Message: "exactly one variant must be set, found 0", Rule: RuleOneOf}},
		{name: "Exactly one with two", check: ExactlyOne, set: []bool{true, true}, want: ValidationError{Message: "exactly one variant must be set, found 2", Rule: RuleOneOf}},
		{name: "At most one with none", check: AtMostOne, set: []bool{false, false}},
		{name: "At most one with two", check: AtMostOne, set: []bool{true, true}, want: ValidationError{Message: "at most one variant may be set, found 2", Rule: RuleOneOf}},
		{name: "At least one with two", check: AtLeastOne, set: []bool{true, true}},
		{name: "At least one with none", check: AtLeastOne, set: []bool{false}, want: ValidationError{Message: "at least one variant must be set", Rule: RuleAnyOf}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.check(tc.set...))
		})
	}
}
