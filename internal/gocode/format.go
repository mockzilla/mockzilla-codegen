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
	"go/token"
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

	line := locate(err, "", src, 0)
	return src, fmt.Errorf("%w: %w\n%s", ErrFormat, err, excerpt(string(src), line))
}

// locate gives every syntax error in err its place in src, which a line directive in src would
// move, and name as its file. skip is the length of what the parser read before src. It returns
// the line of the first error, 0 when err holds none.
func locate(err error, name string, src []byte, skip int) int {
	var list scanner.ErrorList
	if !errors.As(err, &list) || len(list) == 0 {
		return 0
	}

	file := token.NewFileSet().AddFile(name, -1, len(src))
	file.SetLinesForContent(src)
	for _, e := range list {
		e.Pos = file.Position(file.Pos(e.Pos.Offset - skip))
	}
	return list[0].Pos.Line
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
