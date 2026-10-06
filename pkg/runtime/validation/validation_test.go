// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package validation

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		errs Errors
		want string
	}{
		{name: "Empty", want: ""},
		{name: "One without a field", errs: Errors{{Message: "is required"}}, want: "is required"},
		{
			name: "Several",
			errs: Errors{{Field: "name", Message: "is required"}, {Field: "age", Message: "must be positive"}},
			want: "name: is required; age: must be positive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.errs.Error())
		})
	}
}

func TestErrorsErr(t *testing.T) {
	t.Parallel()

	var errs Errors
	require.NoError(t, errs.Err())

	errs.Required("name")
	err := fmt.Errorf("create pet: %w", errs.Err())

	var many *Errors
	require.ErrorAs(t, err, &many)
	assert.Equal(t, errs, *many)

	var one Error
	require.ErrorAs(t, err, &one)
	assert.Equal(t, Error{Field: "name", Message: "is required", Rule: RuleRequired}, one)
}

func TestErrorsAdd(t *testing.T) {
	t.Parallel()

	var errs Errors
	errs.Add("name", "is taken")
	assert.Equal(t, Errors{{Field: "name", Message: "is taken"}}, errs)
}

func TestErrorsAppend(t *testing.T) {
	t.Parallel()

	nested := Errors{{Field: "name", Message: "is required"}, {Field: "[0]", Message: "is empty"}, {Message: "is invalid"}}
	tests := []struct {
		name   string
		prefix string
		err    error
		want   Errors
	}{
		{name: "Nil error", prefix: "pet"},
		{
			name:   "Nested errors get the prefix",
			prefix: "pet",
			err:    &nested,
			want: Errors{
				{Field: "pet.name", Message: "is required"},
				{Field: "pet[0]", Message: "is empty"},
				{Field: "pet", Message: "is invalid"},
			},
		},
		{name: "No prefix keeps the fields", err: &nested, want: nested},
		{
			name:   "Single validation error",
			prefix: "items[2]",
			err:    Error{Field: "name", Message: "is required"},
			want:   Errors{{Field: "items[2].name", Message: "is required"}},
		},
		{
			name:   "Wrapped validation errors",
			prefix: "pet",
			err:    fmt.Errorf("decode: %w", &Errors{{Field: "id", Message: "is required"}}),
			want:   Errors{{Field: "pet.id", Message: "is required"}},
		},
		{
			name:   "Rule and limit are kept",
			prefix: "name",
			err:    MinLength("", 2),
			want:   Errors{{Field: "name", Message: "must be at least 2 characters long", Rule: RuleMinLength, Limit: 2}},
		},
		{
			name:   "Other error",
			prefix: "born",
			err:    errors.New("invalid date"),
			want:   Errors{{Field: "born", Message: "invalid date"}},
		},
		{
			name: "Other error without a prefix",
			err:  errors.New("bad"),
			want: Errors{{Message: "bad"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var errs Errors
			errs.Append(tt.prefix, tt.err)
			assert.Equal(t, tt.want, errs)
		})
	}
}

func TestFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "Wrapped errors", err: fmt.Errorf("decode: %w", &Errors{{Field: "id", Message: "is required"}}), want: true},
		{name: "One error alone", err: Error{Field: "id", Message: "is required"}},
		{name: "Other error", err: errors.New("bad")},
		{name: "No error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Failed(tt.err))
		})
	}
}
