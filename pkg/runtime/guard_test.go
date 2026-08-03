// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportsGeneratorV1(t *testing.T) {
	t.Parallel()

	assert.True(t, SupportsGeneratorV1)
}
