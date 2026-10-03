// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The coverage profile read into blocks.

package main

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
)

const maxLineBytes = 1 << 20

type block struct {
	file      string
	startLine int
	startCol  int
	endLine   int
	endCol    int
	stmts     int
	count     int
}

func parseProfile(r io.Reader) ([]block, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	if !sc.Scan() || !strings.HasPrefix(sc.Text(), "mode: ") {
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("read profile: %w", err)
		}
		return nil, errNoMode
	}

	merged := map[block]int{}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}

		b, err := parseLine(line)
		if err != nil {
			return nil, err
		}

		key := b
		key.count = 0
		merged[key] = max(merged[key], b.count)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}

	blocks := make([]block, 0, len(merged))
	for key, count := range merged {
		key.count = count
		blocks = append(blocks, key)
	}
	slices.SortFunc(blocks, compareBlocks)

	return blocks, nil
}

func parseLine(line string) (block, error) {
	colon := strings.LastIndexByte(line, ':')
	if colon < 0 {
		return block{}, fmt.Errorf("%w: %q", errBadLine, line)
	}

	b := block{file: line[:colon]}
	_, err := fmt.Sscanf(line[colon+1:], "%d.%d,%d.%d %d %d",
		&b.startLine, &b.startCol, &b.endLine, &b.endCol, &b.stmts, &b.count)
	if err != nil {
		return block{}, fmt.Errorf("%w: %q", errBadLine, line)
	}

	return b, nil
}

func compareBlocks(a, b block) int {
	return cmp.Or(
		cmp.Compare(a.file, b.file),
		cmp.Compare(a.startLine, b.startLine),
		cmp.Compare(a.startCol, b.startCol),
		cmp.Compare(a.endLine, b.endLine),
		cmp.Compare(a.endCol, b.endCol),
	)
}
