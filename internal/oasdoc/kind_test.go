// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSection(t *testing.T) {
	t.Parallel()

	for k := KindNone; k <= KindSecurityScheme; k++ {
		if s := k.Section(); s != "" {
			assert.Equal(t, k, SectionKind(s), s)
		}
	}
	assert.Empty(t, KindOperation.Section())
	assert.Equal(t, KindNone, SectionKind("tags"))
}
