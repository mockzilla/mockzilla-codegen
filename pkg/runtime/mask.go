// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// maskText hides the length of a value as well as its content.
	maskText = "********"
	hashText = 16
)

// MaskFull replaces s with a mask of fixed length.
func MaskFull[S ~string](S) S {
	return maskText
}

// MaskRegex replaces each character of the parts of s that re matches with *.
func MaskRegex[S ~string](s S, re *regexp.Regexp) S {
	return S(re.ReplaceAllStringFunc(string(s), func(m string) string {
		return strings.Repeat("*", utf8.RuneCountInString(m))
	}))
}

// MaskHash replaces s with the start of its SHA-256 in hex, so equal values stay equal in logs.
func MaskHash[S ~string](s S) S {
	sum := sha256.Sum256([]byte(s))
	return S(hex.EncodeToString(sum[:])[:hashText])
}

// MaskPartial keeps prefix characters at the start of s and suffix at the end, and masks the rest.
// A value too short to hide anything is masked in full.
func MaskPartial[S ~string](s S, prefix, suffix int) S {
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

// MaskSlice returns a copy of s with each item masked.
func MaskSlice[T interface{ Masked() T }](s []T) []T {
	if s == nil {
		return nil
	}
	out := make([]T, len(s))
	for i, v := range s {
		out[i] = v.Masked()
	}
	return out
}

// MaskMap returns a copy of m with each value masked.
func MaskMap[T interface{ Masked() T }](m map[string]T) map[string]T {
	if m == nil {
		return nil
	}
	out := make(map[string]T, len(m))
	for k, v := range m {
		out[k] = v.Masked()
	}
	return out
}

// LogValue gives slog the JSON of v: an object is a group with its keys in order, an array a list,
// anything else its value. A value without JSON gives the error text.
func LogValue(v any) slog.Value {
	data, err := json.Marshal(v)
	if err != nil {
		return slog.StringValue(err.Error())
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return logValue(dec)
}

// logValue reads the next value from dec, which holds valid JSON.
func logValue(dec *json.Decoder) slog.Value {
	tok, _ := dec.Token()
	switch t := tok.(type) {
	case json.Delim:
		if t == '{' {
			var attrs []slog.Attr
			for dec.More() {
				key, _ := dec.Token()
				name, _ := key.(string)
				attrs = append(attrs, slog.Attr{Key: name, Value: logValue(dec)})
			}
			_, _ = dec.Token()
			return slog.GroupValue(attrs...)
		}
		var items []any
		for dec.More() {
			items = append(items, logValue(dec).Any())
		}
		_, _ = dec.Token()
		return slog.AnyValue(items)
	case string:
		return slog.StringValue(t)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return slog.Int64Value(n)
		}
		f, _ := t.Float64()
		return slog.Float64Value(f)
	case bool:
		return slog.BoolValue(t)
	}
	return slog.AnyValue(nil)
}
