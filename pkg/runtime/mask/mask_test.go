// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package mask

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

type email string

// secret stands in for a generated type with a Masked method.
type secret struct {
	Value string `json:"value"`
}

func (s secret) Masked() secret {
	return secret{Value: Full(s.Value)}
}

func TestMasks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  email
		want email
	}{
		{name: "Full hides the length", got: Full(email("a@b.c")), want: "********"},
		{name: "Regex masks each matched character", got: Regex(email("123-45-67é9"), regexp.MustCompile(`[\dé]`)), want: "***-**-****"},
		{name: "Hash", got: Hash(email("secret")), want: "2bb80d537b1da3e3"},
		{name: "Partial keeps the ends", got: Partial(email("1234567890"), 2, 4), want: "12********7890"},
		{name: "Partial of a short value masks all", got: Partial(email("12345"), 2, 3), want: "********"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.got)
		})
	}
}

func TestCollections(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, Zero(42))
	assert.Nil(t, Zero(new(1)))
	assert.Nil(t, Slice([]secret(nil)))
	assert.Equal(t, []secret{{Value: "********"}}, Slice([]secret{{Value: "a"}}))
	assert.Nil(t, Map(map[string]secret(nil)))
	assert.Equal(t, map[string]secret{"k": {Value: "********"}}, Map(map[string]secret{"k": {Value: "a"}}))
}
