// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithDebug(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	New(WithDebug(&buf)).logger.Debug("hello")
	assert.Contains(t, buf.String(), "msg=hello")
}
