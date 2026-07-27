// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import "testing"

func TestCollector(t *testing.T) {
	t.Parallel()

	reserved := testOptions()
	reserved.Reserved = []string{"Client"}
	reserved.OperationSuffixes = []string{"ResponseData"}
	tests := []struct {
		name    string
		fixture string
		opts    Options
	}{
		{name: "operations", fixture: "operations", opts: testOptions()},
		{name: "operations-reserved", fixture: "operations", opts: reserved},
		{name: "refs", fixture: "refs", opts: testOptions()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checkGolden(t, tt.fixture, tt.name, tt.opts)
		})
	}
}
