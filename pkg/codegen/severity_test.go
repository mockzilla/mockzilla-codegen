// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeverityNames(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "info", SeverityInfo.String())
	assert.Equal(t, "warning", SeverityWarning.String())
	assert.Equal(t, "error", SeverityError.String())
}
