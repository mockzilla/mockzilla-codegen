// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package framework

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckClean(t *testing.T) {
	t.Parallel()

	require.NoError(t, CheckClean("/pets/{id}/"))
	require.NoError(t, CheckClean("/"))
	require.EqualError(t, CheckClean("/a//b"), "the router rejects the path: it is not a clean path")
	require.ErrorIs(t, CheckClean("/a/./b"), ErrPattern)
}
