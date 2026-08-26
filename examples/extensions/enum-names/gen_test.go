// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package enumnames

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnumNames(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Region("eu-west-1"), RegionEurope)
	require.NoError(t, PriorityUrgent.Validate())
	require.EqualError(t, Priority(5).Validate(), "must be one of 0, 1, 2")
}
