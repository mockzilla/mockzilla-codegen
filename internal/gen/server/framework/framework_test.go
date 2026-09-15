// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package framework

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParams(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"owner", "id"}, Params("/owners/{owner}/pets/{id}.json"))
	assert.Nil(t, Params("/pets"))
}

func TestShape(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "GET /owners/{}/pets/{}", Shape("GET /owners/{owner}/pets/{id}"))
	assert.Equal(t, "/files/{}", Shape("/files/{path...}"))
}
