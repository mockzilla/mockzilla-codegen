// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "Top-level message", err: NewSimpleError("not found"), want: "not found"},
		{name: "Nested message", err: NewNestedError("bad input"), want: "bad input"},
		{name: "First item of an array", err: NewListError("too long"), want: "too long"},
		{name: "Union variant that has the path", err: EitherError{Option1: &EitherErrorOption1{Title: "gone"}}, want: "gone"},
		{name: "Union variant without the path", err: EitherError{Option2: &EitherErrorOption2{Reason: "why"}}, want: "EitherError"},
		{name: "Empty nested message", err: NestedError{}, want: "NestedError"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.err.Error())
		})
	}
}

func TestErrorsAs(t *testing.T) {
	t.Parallel()

	var err error = NewSimpleError("not found")
	var simple SimpleError

	require.ErrorAs(t, err, &simple)
	assert.Equal(t, SimpleError{Message: "not found"}, simple)
	assert.False(t, errors.As(err, new(ListError)))
}
