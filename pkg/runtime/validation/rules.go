// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// One check per keyword of the spec: lengths, patterns, formats, limits and enums.

package validation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// hexDigits are the characters a UUID holds between its dashes.
const hexDigits = "0123456789abcdefABCDEF"

// Number is every Go number type.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// Base64 is the text encoding/json writes for b, which the length and pattern of format byte count.
func Base64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// MinLength checks that s has at least n characters.
func MinLength[S ~string](s S, n int) error {
	if utf8.RuneCountInString(string(s)) < n {
		return Error{Message: "must be at least " + strconv.Itoa(n) + " characters long", Rule: RuleMinLength, Limit: n}
	}
	return nil
}

// NotNull checks that n, a value the spec does not let be null, is not null.
func NotNull[N interface{ IsNull() bool }](n N) error {
	if n.IsNull() {
		return Error{Message: "must not be null", Rule: RuleType}
	}
	return nil
}

// MaxLength checks that s has at most n characters.
func MaxLength[S ~string](s S, n int) error {
	if utf8.RuneCountInString(string(s)) > n {
		return Error{Message: "must be at most " + strconv.Itoa(n) + " characters long", Rule: RuleMaxLength, Limit: n}
	}
	return nil
}

// Pattern checks that re, compiled from the spec's pattern, matches s.
func Pattern[S ~string](s S, re *regexp.Regexp, pattern string) error {
	if !re.MatchString(string(s)) {
		return Error{Message: "must match " + pattern, Rule: RulePattern, Limit: pattern}
	}
	return nil
}

// Format checks that s is written in format. Formats it does not know pass.
func Format[S ~string](s S, format string) error {
	var isValid func(string) bool
	switch strings.ToLower(format) {
	case "uuid":
		isValid = isUUID
	case "uri":
		isValid = isURI
	case "uri-reference":
		isValid = isURIReference
	case "ipv4":
		isValid = isIPv4
	case "ipv6":
		isValid = isIPv6
	case "hostname":
		isValid = isHostname
	case "email":
		isValid = isEmail
	case "date":
		isValid = isDate
	case "date-time":
		isValid = isDateTime
	default:
		return nil
	}
	if !isValid(string(s)) {
		return Error{Message: "must be a valid " + format, Rule: RuleFormat, Limit: format}
	}
	return nil
}

// Minimum checks that v is at least bound, or above it when isExclusive.
func Minimum[T Number](v T, bound float64, isExclusive bool) error {
	switch f := float64(v); {
	case isExclusive && f <= bound:
		return Error{Message: "must be greater than " + formatFloat(bound), Rule: RuleExclusiveMinimum, Limit: bound}
	case f < bound:
		return Error{Message: "must be at least " + formatFloat(bound), Rule: RuleMinimum, Limit: bound}
	}
	return nil
}

// Maximum checks that v is at most bound, or below it when isExclusive.
func Maximum[T Number](v T, bound float64, isExclusive bool) error {
	switch f := float64(v); {
	case isExclusive && f >= bound:
		return Error{Message: "must be less than " + formatFloat(bound), Rule: RuleExclusiveMaximum, Limit: bound}
	case f > bound:
		return Error{Message: "must be at most " + formatFloat(bound), Rule: RuleMaximum, Limit: bound}
	}
	return nil
}

// MultipleOf checks that v, as the decimal JSON carries, is a whole multiple of a positive factor.
func MultipleOf[T Number](v T, factor float64) error {
	f, isFactor := new(big.Rat).SetString(formatFloat(factor))
	if !isFactor || f.Sign() <= 0 {
		return nil
	}
	q, isNumber := new(big.Rat).SetString(decimal(v))
	if !isNumber || !q.Quo(q, f).IsInt() {
		return Error{Message: "must be a multiple of " + formatFloat(factor), Rule: RuleMultipleOf, Limit: factor}
	}
	return nil
}

// MinItems checks that s has at least n items.
func MinItems[T any](s []T, n int) error {
	if len(s) < n {
		return Error{Message: "must have at least " + strconv.Itoa(n) + " items", Rule: RuleMinItems, Limit: n}
	}
	return nil
}

// MaxItems checks that s has at most n items.
func MaxItems[T any](s []T, n int) error {
	if len(s) > n {
		return Error{Message: "must have at most " + strconv.Itoa(n) + " items", Rule: RuleMaxItems, Limit: n}
	}
	return nil
}

// Unique checks that no two items of s are equal.
func Unique[T comparable](s []T) error {
	seen := make(map[T]bool, len(s))
	for _, v := range s {
		if seen[v] {
			return Error{Message: "must have unique items", Rule: RuleUniqueItems, Limit: true}
		}
		seen[v] = true
	}
	return nil
}

// UniqueJSON checks that no two items of s have the same JSON, for items Go cannot compare.
func UniqueJSON[T any](s []T) error {
	seen := make(map[string]bool, len(s))
	for _, v := range s {
		data, err := json.Marshal(v)
		if err != nil {
			continue
		}
		if seen[string(data)] {
			return Error{Message: "must have unique items", Rule: RuleUniqueItems, Limit: true}
		}
		seen[string(data)] = true
	}
	return nil
}

// MinProperties checks that m has at least n keys.
func MinProperties[V any](m map[string]V, n int) error {
	if len(m) < n {
		return Error{Message: "must have at least " + strconv.Itoa(n) + " properties", Rule: RuleMinProperties, Limit: n}
	}
	return nil
}

// MaxProperties checks that m has at most n keys.
func MaxProperties[V any](m map[string]V, n int) error {
	if len(m) > n {
		return Error{Message: "must have at most " + strconv.Itoa(n) + " properties", Rule: RuleMaxProperties, Limit: n}
	}
	return nil
}

// Const checks that v is want.
func Const[T comparable](v, want T) error {
	if v != want {
		return Error{Message: fmt.Sprintf("must be %v", want), Rule: RuleConst, Limit: want}
	}
	return nil
}

// Enum checks that v is one of values.
func Enum[T comparable](v T, values ...T) error {
	if slices.Contains(values, v) {
		return nil
	}
	texts := make([]string, len(values))
	for i, value := range values {
		texts[i] = fmt.Sprint(value)
	}
	return Error{Message: "must be one of " + strings.Join(texts, ", "), Rule: RuleEnum, Limit: values}
}

// EnumJSON checks that v is one of values, each read into a T, by comparing both written as JSON.
func EnumJSON[T any](v T, values ...string) error {
	// A value Go cannot write as JSON is none of them.
	got, _ := json.Marshal(v)
	listed := make([]T, 0, len(values))
	for _, entry := range values {
		var want T
		if json.Unmarshal([]byte(entry), &want) != nil {
			continue
		}
		data, wantErr := json.Marshal(want)
		if wantErr == nil && sameJSON(got, data) {
			return nil
		}
		listed = append(listed, want)
	}
	return Error{Message: "must be one of " + strings.Join(values, ", "), Rule: RuleEnum, Limit: listed}
}

// Index is the path of item i under path: items[2].
func Index(path string, i int) string {
	return path + "[" + strconv.Itoa(i) + "]"
}

// Key is the path of the value under key: labels["team"], or team at the top.
func Key(path, key string) string {
	if path == "" {
		return key
	}
	return path + "[" + strconv.Quote(key) + "]"
}

// SortedKeys returns the keys of m in order, so errors come out the same on every run.
func SortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}

// IsZero reports whether v is nil or its zero value, by its own IsZero method too, as omitzero sees it.
func IsZero(v any) bool {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || rv.IsZero() {
		return true
	}
	z, ok := v.(interface{ IsZero() bool })
	return ok && z.IsZero()
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// decimal is the shortest text that reads back as v in its own type.
func decimal[T Number](v T) string {
	switch r := reflect.ValueOf(v); {
	case r.CanInt():
		return strconv.FormatInt(r.Int(), 10)
	case r.CanUint():
		return strconv.FormatUint(r.Uint(), 10)
	default:
		return strconv.FormatFloat(r.Float(), 'g', -1, r.Type().Bits())
	}
}

// sameJSON reports whether a and b hold the same JSON value; numbers are the same when equal.
func sameJSON(a, b []byte) bool {
	x, errX := decodeNumbers(a)
	y, errY := decodeNumbers(b)
	return errX == nil && errY == nil && sameValue(x, y)
}

func decodeNumbers(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	err := dec.Decode(&v)
	return v, err
}

func sameValue(x, y any) bool {
	switch x := x.(type) {
	case json.Number:
		n, ok := y.(json.Number)
		if !ok {
			return false
		}
		a, isA := new(big.Rat).SetString(x.String())
		b, isB := new(big.Rat).SetString(n.String())
		return isA && isB && a.Cmp(b) == 0
	case []any:
		items, ok := y.([]any)
		return ok && slices.EqualFunc(x, items, sameValue)
	case map[string]any:
		fields, ok := y.(map[string]any)
		if !ok || len(x) != len(fields) {
			return false
		}
		for key, value := range x {
			if other, has := fields[key]; !has || !sameValue(value, other) {
				return false
			}
		}
		return true
	default:
		return x == y
	}
}
