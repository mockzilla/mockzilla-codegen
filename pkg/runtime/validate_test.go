// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"math"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type level string

func TestChecksOfValues(t *testing.T) {
	t.Parallel()

	letters := regexp.MustCompile(`^[a-z]+$`)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "Min length counts characters", err: MinLength("éé", 2)},
		{name: "Too short", err: MinLength(level("a"), 2), want: "must be at least 2 characters long"},
		{name: "Max length", err: MaxLength("abc", 3)},
		{name: "Too long", err: MaxLength("abcd", 3), want: "must be at most 3 characters long"},
		{name: "Base64 counts the encoded text", err: MaxLength(Base64([]byte("hello!!")), 8), want: "must be at most 8 characters long"},
		{name: "Pattern matches", err: Pattern("abc", letters, `^\p{Ll}+$`)},
		{name: "Pattern fails with the spec's text", err: Pattern("ab1", letters, `^\p{Ll}+$`), want: `must match ^\p{Ll}+$`},
		{name: "Format passes", err: Format("0f8fad5b-d9cb-469f-a165-70867728950e", "uuid")},
		{name: "Format fails", err: Format("x", "UUID"), want: "must be a valid UUID"},
		{name: "Unknown format passes", err: Format("x", "color")},
		{name: "Minimum", err: Minimum(3, 3, false)},
		{name: "Below minimum", err: Minimum(int8(2), 2.5, false), want: "must be at least 2.5"},
		{name: "At exclusive minimum", err: Minimum(3.0, 3, true), want: "must be greater than 3"},
		{name: "Maximum", err: Maximum(uint(3), 3, false)},
		{name: "Above maximum", err: Maximum(4, 3, false), want: "must be at most 3"},
		{name: "At exclusive maximum", err: Maximum(3, 3, true), want: "must be less than 3"},
		{name: "Multiple of", err: MultipleOf(0.3, 0.1)},
		{name: "No multiple", err: MultipleOf(7, 2), want: "must be a multiple of 2"},
		{name: "Factor that is not positive", err: MultipleOf(7, 0)},
		{name: "Factor that is no number", err: MultipleOf(7, math.NaN())},
		{name: "Multiple of a cent", err: MultipleOf(1234567.89, 0.01)},
		{name: "Float32 read as written", err: MultipleOf(float32(0.07), 0.01)},
		{name: "Unsigned multiple", err: MultipleOf(uint8(6), 3)},
		{name: "Integer past float64", err: MultipleOf(int64(9007199254740993), 2), want: "must be a multiple of 2"},
		{name: "Near miss as written", err: MultipleOf(0.30000000000000004, 0.01), want: "must be a multiple of 0.01"},
		{name: "NaN is no multiple", err: MultipleOf(math.NaN(), 0.1), want: "must be a multiple of 0.1"},
		{name: "Min items", err: MinItems([]int{1}, 1)},
		{name: "Too few items", err: MinItems([]int{}, 1), want: "must have at least 1 items"},
		{name: "Max items", err: MaxItems([]int{1}, 1)},
		{name: "Too many items", err: MaxItems([]int{1, 2}, 1), want: "must have at most 1 items"},
		{name: "Unique", err: Unique([]string{"a", "b"})},
		{name: "Repeated item", err: Unique([]string{"a", "a"}), want: "must have unique items"},
		{name: "Unique by JSON", err: UniqueJSON([][]int{{1}, {2}})},
		{name: "Repeated item by JSON", err: UniqueJSON([][]int{{1}, {1}}), want: "must have unique items"},
		{name: "Item that has no JSON is skipped", err: UniqueJSON([]any{func() {}, func() {}})},
		{name: "Min properties", err: MinProperties(map[string]int{"a": 1}, 1)},
		{name: "Too few properties", err: MinProperties(map[string]int{}, 1), want: "must have at least 1 properties"},
		{name: "Max properties", err: MaxProperties(map[string]int{"a": 1}, 1)},
		{name: "Too many properties", err: MaxProperties(map[string]int{"a": 1, "b": 2}, 1), want: "must have at most 1 properties"},
		{name: "Const", err: Const("a", "a")},
		{name: "Not the const", err: Const(2, 3), want: "must be 3"},
		{name: "One of", err: OneOf(level("b"), "a", "b")},
		{name: "None of", err: OneOf(level("c"), "a", "b"), want: "must be one of a, b"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.want == "" {
				require.NoError(t, tc.err)
				return
			}
			assert.Equal(t, ValidationError{Message: tc.want}, tc.err)
		})
	}
}

func TestPaths(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "items[2]", Index("items", 2))
	assert.Equal(t, `labels["a b"]`, Key("labels", "a b"))
	assert.Equal(t, "team", Key("", "team"))
	assert.Equal(t, []string{"a", "b", "c"}, SortedKeys(map[string]int{"c": 1, "a": 2, "b": 3}))
}
