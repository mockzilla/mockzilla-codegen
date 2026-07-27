// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationErrorsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		errs ValidationErrors
		want string
	}{
		{name: "Empty", want: ""},
		{name: "One without a field", errs: ValidationErrors{{Message: "is required"}}, want: "is required"},
		{
			name: "Several",
			errs: ValidationErrors{{Field: "name", Message: "is required"}, {Field: "age", Message: "must be positive"}},
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

func TestValidationErrorsErr(t *testing.T) {
	t.Parallel()

	var errs ValidationErrors
	require.NoError(t, errs.Err())

	errs.Add("name", "is required")
	err := fmt.Errorf("create pet: %w", errs.Err())

	var many ValidationErrors
	require.ErrorAs(t, err, &many)
	assert.Equal(t, errs, many)

	var one ValidationError
	require.ErrorAs(t, err, &one)
	assert.Equal(t, ValidationError{Field: "name", Message: "is required"}, one)
}

func TestValidationErrorsAppend(t *testing.T) {
	t.Parallel()

	nested := ValidationErrors{{Field: "name", Message: "is required"}, {Field: "[0]", Message: "is empty"}, {Message: "is invalid"}}
	tests := []struct {
		name   string
		prefix string
		err    error
		want   ValidationErrors
	}{
		{name: "Nil error", prefix: "pet"},
		{
			name:   "Nested errors get the prefix",
			prefix: "pet",
			err:    nested,
			want: ValidationErrors{
				{Field: "pet.name", Message: "is required"},
				{Field: "pet[0]", Message: "is empty"},
				{Field: "pet", Message: "is invalid"},
			},
		},
		{name: "No prefix keeps the fields", err: nested, want: nested},
		{
			name:   "Single validation error",
			prefix: "items[2]",
			err:    ValidationError{Field: "name", Message: "is required"},
			want:   ValidationErrors{{Field: "items[2].name", Message: "is required"}},
		},
		{
			name:   "Wrapped validation errors",
			prefix: "pet",
			err:    fmt.Errorf("decode: %w", ValidationErrors{{Field: "id", Message: "is required"}}),
			want:   ValidationErrors{{Field: "pet.id", Message: "is required"}},
		},
		{
			name:   "Other error",
			prefix: "email",
			err:    ErrInvalidEmail,
			want:   ValidationErrors{{Field: "email", Message: "invalid email address"}},
		},
		{
			name: "Other error without a prefix",
			err:  errors.New("bad"),
			want: ValidationErrors{{Message: "bad"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var errs ValidationErrors
			errs.Append(tt.prefix, tt.err)
			assert.Equal(t, tt.want, errs)
		})
	}
}
