// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// A JSON object that keeps its keys in the order they were set, so a schema reads the same on
// every run.

package jsonschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// Object is a JSON object whose keys keep the order they were first set in. Values are nil, bool,
// string, json.Number, []string, []any or *Object; anything else is written as null.
type Object struct {
	keys   []string
	values map[string]any
}

// Set sets key to v, keeping the position of a key that is set again.
func (o *Object) Set(key string, v any) *Object {
	if o.values == nil {
		o.values = map[string]any{}
	}
	if !slices.Contains(o.keys, key) {
		o.keys = append(o.keys, key)
	}
	o.values[key] = v
	return o
}

// Len is the number of keys set.
func (o *Object) Len() int {
	return len(o.keys)
}

// MarshalJSON writes the object compact, with HTML characters left as they are.
func (o *Object) MarshalJSON() ([]byte, error) {
	return o.append(nil), nil
}

func (o *Object) append(buf []byte) []byte {
	buf = append(buf, '{')
	for i, k := range o.keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = appendString(buf, k)
		buf = append(buf, ':')
		buf = appendValue(buf, o.values[k])
	}
	return append(buf, '}')
}

func appendValue(buf []byte, v any) []byte {
	switch v := v.(type) {
	case bool:
		return append(buf, strconv.FormatBool(v)...)
	case string:
		return appendString(buf, v)
	case json.Number:
		if json.Valid([]byte(v)) {
			return append(buf, v...)
		}
		return appendString(buf, string(v))
	case []string:
		return appendList(buf, len(v), func(i int, out []byte) []byte { return appendString(out, v[i]) })
	case []any:
		return appendList(buf, len(v), func(i int, out []byte) []byte { return appendValue(out, v[i]) })
	case *Object:
		return v.append(buf)
	}
	return append(buf, "null"...)
}

func appendList(buf []byte, n int, item func(i int, out []byte) []byte) []byte {
	buf = append(buf, '[')
	for i := range n {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = item(i, buf)
	}
	return append(buf, ']')
}

// appendString quotes s as JSON with HTML characters as they are and unprintable ones escaped.
func appendString(buf []byte, s string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	// Encoding a string cannot fail: invalid UTF-8 is replaced.
	_ = enc.Encode(s)

	// gocode.RawString quotes a whole schema when one of its characters is not printable.
	for _, r := range strings.TrimSuffix(b.String(), "\n") {
		if unicode.IsPrint(r) {
			buf = utf8.AppendRune(buf, r)
			continue
		}
		for _, unit := range utf16.Encode([]rune{r}) {
			buf = fmt.Appendf(buf, `\u%04x`, unit)
		}
	}
	return buf
}
