// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package validation

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

type level string

type shade struct {
	R int  `json:"r"`
	G *int `json:"g,omitempty"`
}

type noJSON struct{}

func (noJSON) MarshalJSON() ([]byte, error) {
	return nil, errors.New("no JSON")
}

type nullable bool

func (n nullable) IsNull() bool {
	return bool(n)
}

func TestChecksOfValues(t *testing.T) {
	t.Parallel()

	letters := regexp.MustCompile(`^[a-z]+$`)
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "Min length counts characters", err: MinLength("éé", 2)},
		{name: "Too short", err: MinLength(level("a"), 2), want: Error{Message: "must be at least 2 characters long", Rule: RuleMinLength, Limit: 2}},
		{name: "Max length", err: MaxLength("abc", 3)},
		{name: "Too long", err: MaxLength("abcd", 3), want: Error{Message: "must be at most 3 characters long", Rule: RuleMaxLength, Limit: 3}},
		{name: "A value is not null", err: NotNull(nullable(false))},
		{name: "Null", err: NotNull(nullable(true)), want: Error{Message: "must not be null", Rule: RuleType}},
		{name: "Base64 counts the encoded text", err: MaxLength(Base64([]byte("hello!!")), 8), want: Error{Message: "must be at most 8 characters long", Rule: RuleMaxLength, Limit: 8}},
		{name: "Pattern matches", err: Pattern("abc", letters, `^\p{Ll}+$`)},
		{name: "Pattern fails with the spec's text", err: Pattern("ab1", letters, `^\p{Ll}+$`), want: Error{Message: `must match ^\p{Ll}+$`, Rule: RulePattern, Limit: `^\p{Ll}+$`}},
		{name: "Format passes", err: Format("0f8fad5b-d9cb-469f-a165-70867728950e", "uuid")},
		{name: "Format fails", err: Format("x", "UUID"), want: Error{Message: "must be a valid UUID", Rule: RuleFormat, Limit: "UUID"}},
		{name: "Unknown format passes", err: Format("x", "color")},
		{name: "Minimum", err: Minimum(3, 3, false)},
		{name: "Below minimum", err: Minimum(int8(2), 2.5, false), want: Error{Message: "must be at least 2.5", Rule: RuleMinimum, Limit: 2.5}},
		{name: "At exclusive minimum", err: Minimum(3.0, 3, true), want: Error{Message: "must be greater than 3", Rule: RuleExclusiveMinimum, Limit: 3.0}},
		{name: "Maximum", err: Maximum(uint(3), 3, false)},
		{name: "Above maximum", err: Maximum(4, 3, false), want: Error{Message: "must be at most 3", Rule: RuleMaximum, Limit: 3.0}},
		{name: "At exclusive maximum", err: Maximum(3, 3, true), want: Error{Message: "must be less than 3", Rule: RuleExclusiveMaximum, Limit: 3.0}},
		{name: "Multiple of", err: MultipleOf(0.3, 0.1)},
		{name: "No multiple", err: MultipleOf(7, 2), want: Error{Message: "must be a multiple of 2", Rule: RuleMultipleOf, Limit: 2.0}},
		{name: "Factor that is not positive", err: MultipleOf(7, 0)},
		{name: "Factor that is no number", err: MultipleOf(7, math.NaN())},
		{name: "Multiple of a cent", err: MultipleOf(1234567.89, 0.01)},
		{name: "Float32 read as written", err: MultipleOf(float32(0.07), 0.01)},
		{name: "Unsigned multiple", err: MultipleOf(uint8(6), 3)},
		{name: "Integer past float64", err: MultipleOf(int64(9007199254740993), 2), want: Error{Message: "must be a multiple of 2", Rule: RuleMultipleOf, Limit: 2.0}},
		{name: "Near miss as written", err: MultipleOf(0.30000000000000004, 0.01), want: Error{Message: "must be a multiple of 0.01", Rule: RuleMultipleOf, Limit: 0.01}},
		{name: "NaN is no multiple", err: MultipleOf(math.NaN(), 0.1), want: Error{Message: "must be a multiple of 0.1", Rule: RuleMultipleOf, Limit: 0.1}},
		{name: "Min items", err: MinItems([]int{1}, 1)},
		{name: "Too few items", err: MinItems([]int{}, 1), want: Error{Message: "must have at least 1 items", Rule: RuleMinItems, Limit: 1}},
		{name: "Max items", err: MaxItems([]int{1}, 1)},
		{name: "Too many items", err: MaxItems([]int{1, 2}, 1), want: Error{Message: "must have at most 1 items", Rule: RuleMaxItems, Limit: 1}},
		{name: "Unique", err: Unique([]string{"a", "b"})},
		{name: "Repeated item", err: Unique([]string{"a", "a"}), want: Error{Message: "must have unique items", Rule: RuleUniqueItems, Limit: true}},
		{name: "Unique by JSON", err: UniqueJSON([][]int{{1}, {2}})},
		{name: "Repeated item by JSON", err: UniqueJSON([][]int{{1}, {1}}), want: Error{Message: "must have unique items", Rule: RuleUniqueItems, Limit: true}},
		{name: "Item that has no JSON is skipped", err: UniqueJSON([]any{func() {}, func() {}})},
		{name: "Min properties", err: MinProperties(map[string]int{"a": 1}, 1)},
		{name: "Too few properties", err: MinProperties(map[string]int{}, 1), want: Error{Message: "must have at least 1 properties", Rule: RuleMinProperties, Limit: 1}},
		{name: "Max properties", err: MaxProperties(map[string]int{"a": 1}, 1)},
		{name: "Too many properties", err: MaxProperties(map[string]int{"a": 1, "b": 2}, 1), want: Error{Message: "must have at most 1 properties", Rule: RuleMaxProperties, Limit: 1}},
		{name: "Const", err: Const("a", "a")},
		{name: "Not the const", err: Const(2, 3), want: Error{Message: "must be 3", Rule: RuleConst, Limit: 3}},
		{name: "One of", err: Enum(level("b"), "a", "b")},
		{name: "None of", err: Enum(level("c"), "a", "b"), want: Error{Message: "must be one of a, b", Rule: RuleEnum, Limit: []level{"a", "b"}}},
		{name: "One of by JSON as Go holds it", err: EnumJSON(shade{R: 255}, `{"r":255,"g":null}`, `{"r":0,"g":255}`)},
		{name: "Key order does not count", err: EnumJSON(shade{G: new(255)}, `{"g":255,"r":0}`)},
		{name: "Numbers in any compare by value", err: EnumJSON[any](map[string]any{"a": []any{json.Number("1.50")}}, `{"a":[1.5]}`)},
		{
			name: "None of by JSON leaves out what Go cannot read",
			err:  EnumJSON(shade{R: 1}, `{"r":255}`, `{"r":"x"}`),
			want: Error{Message: `must be one of {"r":255}, {"r":"x"}`, Rule: RuleEnum, Limit: []shade{{R: 255}}},
		},
		{
			name: "Objects differ by keys and values",
			err:  EnumJSON[any](map[string]any{"a": 1}, `[1]`, `{"a":1,"b":2}`, `{"b":1}`, `{"a":2}`),
			want: Error{
				Message: `must be one of [1], {"a":1,"b":2}, {"b":1}, {"a":2}`,
				Rule:    RuleEnum,
				Limit:   []any{[]any{1.0}, map[string]any{"a": 1.0, "b": 2.0}, map[string]any{"b": 1.0}, map[string]any{"a": 2.0}},
			},
		},
		{
			name: "A number is no string and a list no object",
			err:  EnumJSON[any]([]any{json.Number("1")}, `["1"]`, `{"a":1}`),
			want: Error{Message: `must be one of ["1"], {"a":1}`, Rule: RuleEnum, Limit: []any{[]any{"1"}, map[string]any{"a": 1.0}}},
		},
		{name: "Strings compare as written", err: EnumJSON[any]("a", `1`, `"a"`)},
		{
			name: "A value with no JSON is none of them",
			err:  EnumJSON(noJSON{}, `{}`),
			want: Error{Message: "must be one of {}", Rule: RuleEnum, Limit: []noJSON{{}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.err)
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
