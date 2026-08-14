// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import "strconv"

// ExactlyOne is the check of a oneOf union: one variant set.
func ExactlyOne(set ...bool) error {
	if n := count(set); n != 1 {
		return ValidationError{Message: "exactly one variant must be set, found " + strconv.Itoa(n)}
	}
	return nil
}

// AtMostOne is the check of a nullable oneOf union, where nothing set is null.
func AtMostOne(set ...bool) error {
	if n := count(set); n > 1 {
		return ValidationError{Message: "at most one variant may be set, found " + strconv.Itoa(n)}
	}
	return nil
}

// AtLeastOne is the check of an anyOf union.
func AtLeastOne(set ...bool) error {
	if count(set) == 0 {
		return ValidationError{Message: "at least one variant must be set"}
	}
	return nil
}

func count(set []bool) int {
	n := 0
	for _, isSet := range set {
		if isSet {
			n++
		}
	}
	return n
}
