// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package layout

import (
	"bufio"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Module is the Go module the output belongs to. Path is empty when none is known.
type Module struct {
	Path string
	Dir  string
}

// FindModule reads the nearest go.mod in dir or above it. A non-empty override replaces its
// module path; without a go.mod, the module is override rooted at dir.
func FindModule(dir, override string) (Module, error) {
	// Abs fails only when the working directory is gone, so it shares the read error path.
	start, err := filepath.Abs(dir)
	if err == nil {
		var mod Module
		if mod, err = walkUp(start, override); err == nil {
			return mod, nil
		}
	}
	return Module{}, fmt.Errorf("%w: %w", ErrModule, err)
}

func walkUp(start, override string) (Module, error) {
	for d := start; ; d = filepath.Dir(d) {
		file := filepath.Join(d, "go.mod")
		data, err := os.ReadFile(file)
		if err == nil {
			var modPath string
			if modPath, err = modulePath(data); err != nil {
				return Module{}, fmt.Errorf("%s: %w", file, err)
			}
			return Module{Path: cmp.Or(override, modPath), Dir: d}, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return Module{}, err
		}
		if filepath.Dir(d) == d {
			return Module{Path: override, Dir: start}, nil
		}
	}
}

func modulePath(data []byte) (string, error) {
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		rest, ok := strings.CutPrefix(strings.TrimSpace(s.Text()), "module")
		if !ok || rest == "" || !strings.ContainsAny(rest[:1], " \t\"`") {
			continue
		}
		if i := strings.Index(rest, "//"); i >= 0 {
			rest = rest[:i]
		}
		rest = strings.TrimSpace(rest)
		if p, err := strconv.Unquote(rest); err == nil {
			return p, nil
		}
		if rest != "" {
			return rest, nil
		}
	}
	return "", errNoModuleLine
}
