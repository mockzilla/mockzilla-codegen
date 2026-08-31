// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouterMethod(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Get", routerMethod("GET"))
	assert.Equal(t, "Options", routerMethod("OPTIONS"))
}
