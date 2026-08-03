// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"errors"
	"fmt"
	"go/format"
	"go/scanner"
	"strings"
)

const contextLines = 3

// Format runs go/format on src. On failure it returns src unchanged and an error that shows the
// lines around the first problem.
func Format(src []byte) ([]byte, error) {
	out, err := format.Source(src)
	if err == nil {
		return out, nil
	}

	line := 0
	var list scanner.ErrorList
	if errors.As(err, &list) && len(list) > 0 {
		line = list[0].Pos.Line
	}
	return src, fmt.Errorf("%w: %w\n%s", ErrFormat, err, excerpt(string(src), line))
}

// excerpt returns the lines around line, numbered, with the line itself marked. Line 0 gives
// the start of src.
func excerpt(src string, line int) string {
	lines := strings.Split(src, "\n")
	first := max(line-contextLines, 1)
	last := min(line+contextLines, len(lines))

	var b strings.Builder
	for n := first; n <= last; n++ {
		mark := " "
		if n == line {
			mark = ">"
		}
		fmt.Fprintf(&b, "%s%5d | %s\n", mark, n, lines[n-1])
	}
	return b.String()
}
