// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ByteSize is a number of bytes, written as a plain integer or with a unit: 512KB, 32MB, 1GB.
// Units are powers of 1024.
type ByteSize int64

var byteUnits = []struct {
	suffix string
	shift  uint
}{
	{suffix: "GB", shift: 30},
	{suffix: "MB", shift: 20},
	{suffix: "KB", shift: 10},
	{suffix: "B", shift: 0},
}

// UnmarshalText parses a size such as 1024, 512KB or 32MB.
func (s *ByteSize) UnmarshalText(text []byte) error {
	digits, shift := string(text), uint(0)
	for _, u := range byteUnits {
		if rest, ok := strings.CutSuffix(digits, u.suffix); ok {
			digits, shift = rest, u.shift
			break
		}
	}

	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || n < 0 || n > math.MaxInt64>>shift {
		return fmt.Errorf("%w %q, want an integer or a size such as 512KB or 32MB", ErrByteSize, text)
	}

	*s = ByteSize(n << shift)
	return nil
}
