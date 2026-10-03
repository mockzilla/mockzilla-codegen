// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import "testing"

func TestNames(t *testing.T) {
	t.Parallel()

	reserved := testOptions()
	reserved.Reserved = []string{"Client"}
	client := testOptions()
	client.Methods = Methods{Client: []string{"Request"}}
	streams := testOptions()
	streams.Methods = Methods{Client: []string{"Request", "WithResponse"}, Stream: []string{"Stream", "StreamWithResponse"}}
	tools := testOptions()
	tools.Methods = Methods{Tool: []string{"Tool"}, IsToolSkipped: true}
	tests := []struct {
		name    string
		fixture string
		opts    Options
	}{
		{name: "naming", fixture: "naming", opts: reserved},
		{name: "methods", fixture: "methods", opts: testOptions()},
		{name: "methods-client", fixture: "methods", opts: client},
		{name: "methods-streams", fixture: "methods", opts: streams},
		{name: "methods-tools", fixture: "methods", opts: tools},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checkGolden(t, tt.fixture, tt.name, tt.opts)
		})
	}
}
