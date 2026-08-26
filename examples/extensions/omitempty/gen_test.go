// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package omitempty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOmitEmpty(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(Query{})
	require.NoError(t, err)

	assert.JSONEq(t, `{"limit":null}`, string(data))
}
