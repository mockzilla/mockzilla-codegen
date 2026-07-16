// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"regexp"
)

const statusDefault = "default"

// Response keys sort by rank: exact codes, then ranges like 2XX, then anything else, then default.
const (
	rankCode = iota
	rankRange
	rankOther
	rankDefault
)

var (
	statusCode  = regexp.MustCompile(`^[1-5][0-9][0-9]$`)
	statusRange = regexp.MustCompile(`^[1-5][xX][xX]$`)
)

func statusRank(status string) int {
	switch {
	case statusCode.MatchString(status):
		return rankCode
	case statusRange.MatchString(status):
		return rankRange
	case status == statusDefault:
		return rankDefault
	default:
		return rankOther
	}
}
