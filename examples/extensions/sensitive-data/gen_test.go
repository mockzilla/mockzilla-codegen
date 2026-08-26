// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package sensitive

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

func user() User {
	return User{
		Username: "johndoe",
		Password: new("hunter2"),
		Ssn:      "123-45-6789",
		Card:     new("1234-5678-9012-3456"),
		APIKey:   new("my-secret-key"),
		Pin:      new(1234),
		Contacts: []Contact{{Email: new(runtime.Email("a@example.com"))}},
	}
}

func TestMasked(t *testing.T) {
	t.Parallel()

	u := user()
	got := u.Masked()

	assert.Equal(t, User{
		Username: "johndoe",
		Password: new("********"),
		Ssn:      "***-**-****",
		Card:     new("********3456"),
		APIKey:   new("1311f8fc80a7ea28"),
		Contacts: []Contact{{Email: new(runtime.Email("********"))}},
	}, got)
	assert.Equal(t, user(), u, "the original keeps its values")
}

func TestJSONStaysRaw(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(user())
	require.NoError(t, err)

	assert.Contains(t, string(data), `"password":"hunter2"`)
}

func TestLogValue(t *testing.T) {
	t.Parallel()

	var b bytes.Buffer
	slog.New(slog.NewJSONHandler(&b, nil)).Info("created", "user", user())

	assert.Contains(t, b.String(), `"user":{"username":"johndoe","password":"********","ssn":"***-**-****"`)
	assert.NotContains(t, b.String(), "hunter2")
}
