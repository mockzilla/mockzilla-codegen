// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		check func(set ...bool) error
		set   []bool
		want  string
	}{
		{name: "Exactly one with one", check: ExactlyOne, set: []bool{false, true}},
		{name: "Exactly one with none", check: ExactlyOne, set: []bool{false, false}, want: "exactly one variant must be set, found 0"},
		{name: "Exactly one with two", check: ExactlyOne, set: []bool{true, true}, want: "exactly one variant must be set, found 2"},
		{name: "At most one with none", check: AtMostOne, set: []bool{false, false}},
		{name: "At most one with two", check: AtMostOne, set: []bool{true, true}, want: "at most one variant may be set, found 2"},
		{name: "At least one with two", check: AtLeastOne, set: []bool{true, true}},
		{name: "At least one with none", check: AtLeastOne, set: []bool{false}, want: "at least one variant must be set"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.check(tc.set...)

			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			var ve ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, ValidationError{Message: tc.want}, ve)
		})
	}
}
