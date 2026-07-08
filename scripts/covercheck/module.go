// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"strings"
)

func modulePath(gomod string) (string, error) {
	data, err := os.ReadFile(gomod)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}

	for line := range strings.Lines(string(data)) {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(mod), `"`), nil
		}
	}

	return "", errNoModule
}
