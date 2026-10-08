// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package skippointer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZeroValueIsAbsent(t *testing.T) {
	t.Parallel()

	require.NoError(t, Settings{}.Validate())
	require.NoError(t, Settings{FontSize: 12}.Validate())
	require.EqualError(t, Settings{FontSize: 4}.Validate(), "fontSize: must be at least 8")

	data, err := json.Marshal(Settings{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(data))
}
