// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

func loadIgnore(file string) ([]string, error) {
	if file == "" {
		return nil, nil
	}

	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ignore file: %w", err)
	}

	var patterns []string
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if _, err := path.Match(line, ""); err != nil {
			return nil, fmt.Errorf("%w %q: %w", errBadPattern, line, err)
		}
		patterns = append(patterns, line)
	}

	return patterns, nil
}

func ignored(rel string, patterns []string) bool {
	for _, p := range patterns {
		if dir, ok := strings.CutSuffix(p, "/..."); ok {
			if rel == dir || strings.HasPrefix(rel, dir+"/") {
				return true
			}
			continue
		}
		if ok, _ := path.Match(p, rel); ok {
			return true
		}
	}
	return false
}
