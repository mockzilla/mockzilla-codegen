// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import "testing"

func TestBuilder(t *testing.T) {
	t.Parallel()

	tagged := testOptions()
	tagged.ExtraTags = []string{"yaml", "json", "db", "yaml"}
	tagged.Descriptions = false
	tests := []struct {
		name    string
		fixture string
		opts    Options
	}{
		{name: "fields", fixture: "fields", opts: testOptions()},
		{name: "fields-tagged", fixture: "fields", opts: tagged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checkGolden(t, tt.fixture, tt.name, tt.opts)
		})
	}
}
