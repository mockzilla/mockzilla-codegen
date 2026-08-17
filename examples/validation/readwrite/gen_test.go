// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package readwrite

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserSides(t *testing.T) {
	t.Parallel()

	request := User{Name: "Ann", Password: "secret-password"}
	response := User{ID: "0f8fad5b-d9cb-469f-a165-70867728950e", Name: "Ann", Roles: []string{"admin"}}

	require.NoError(t, request.Validate(), "a request leaves out readOnly id and roles")
	require.NoError(t, response.ValidateResponse(), "a response leaves out the writeOnly password")
	require.EqualError(t, request.ValidateResponse(), "id: must be a valid uuid; roles: is required")
	require.EqualError(t, response.Validate(), "password: must be at least 8 characters long")
}

func TestTeamSides(t *testing.T) {
	t.Parallel()

	team := Team{Lead: &User{Name: "Ann"}}

	require.EqualError(t, team.Validate(), "lead.password: must be at least 8 characters long")
	require.EqualError(t, team.ValidateResponse(), "lead.id: must be a valid uuid; lead.roles: is required")
}
