// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import "testing"

func TestPlanBodies(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.IsServer = true
	checkGolden(t, "bodies", "bodies", opts)
}
