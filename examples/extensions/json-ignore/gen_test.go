// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package jsonignore

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIgnoredField(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(Session{ID: new("s1"), Cache: &SessionCache{Hits: new(3)}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"s1"}`, string(data))

	var s Session
	require.NoError(t, json.Unmarshal([]byte(`{"id":"s1","cache":{"hits":3}}`), &s))
	assert.Equal(t, Session{ID: new("s1")}, s)
}
