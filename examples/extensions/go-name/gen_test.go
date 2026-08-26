// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package goname

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNames(t *testing.T) {
	t.Parallel()

	a := Account{Number: new("42"), internalNote: new("kept here")}
	data, err := json.Marshal(a)
	require.NoError(t, err)

	assert.JSONEq(t, `{"acct_no":"42"}`, string(data))
	assert.Equal(t, "kept here", *a.internalNote)
}
