// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package mask hides sensitive values, for logs and other output.
package mask

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// maskText hides the length of a value as well as its content.
	maskText = "********"
	hashText = 16
)

// Full replaces s with a mask of fixed length.
func Full[S ~string](S) S {
	return maskText
}

// Regex replaces each character of the parts of s that re matches with *.
func Regex[S ~string](s S, re *regexp.Regexp) S {
	return S(re.ReplaceAllStringFunc(string(s), func(m string) string {
		return strings.Repeat("*", utf8.RuneCountInString(m))
	}))
}

// Hash replaces s with the start of its SHA-256 in hex, so equal values stay equal in logs.
func Hash[S ~string](s S) S {
	sum := sha256.Sum256([]byte(s))
	return S(hex.EncodeToString(sum[:])[:hashText])
}

// Partial keeps prefix characters at the start of s and suffix at the end, and masks the rest.
// A value too short to hide anything is masked in full.
func Partial[S ~string](s S, prefix, suffix int) S {
	r := []rune(string(s))
	if prefix+suffix >= len(r) {
		return maskText
	}
	return S(string(r[:prefix]) + maskText + string(r[len(r)-suffix:]))
}

// Zero returns the zero value of the type of v, for a sensitive value that is no string.
func Zero[T any](T) T {
	var zero T
	return zero
}

// Slice returns a copy of s with each item masked.
func Slice[T interface{ Masked() T }](s []T) []T {
	if s == nil {
		return nil
	}
	out := make([]T, len(s))
	for i, v := range s {
		out[i] = v.Masked()
	}
	return out
}

// Map returns a copy of m with each value masked.
func Map[T interface{ Masked() T }](m map[string]T) map[string]T {
	if m == nil {
		return nil
	}
	out := make(map[string]T, len(m))
	for k, v := range m {
		out[k] = v.Masked()
	}
	return out
}
