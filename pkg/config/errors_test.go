// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnknownKeyError(t *testing.T) {
	t.Parallel()

	err := &UnknownKeyError{Keys: []string{"specs", "server.port"}}

	require.EqualError(t, err, "unknown config key: specs, server.port")
	assert.ErrorIs(t, err, ErrUnknownKey)
}

func TestValidationError(t *testing.T) {
	t.Parallel()

	err := &ValidationError{Issues: []Issue{
		{Key: "package", Message: "bad"},
		{Key: "server.framework", Message: "required"},
	}}

	require.EqualError(t, err, "invalid config: package: bad; server.framework: required")
	assert.ErrorIs(t, err, ErrInvalid)
}
