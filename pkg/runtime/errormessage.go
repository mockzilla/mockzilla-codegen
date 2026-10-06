// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The message of a generated error type, read from a path in its JSON.

package runtime

import (
	"encoding/json"
	"strings"
)

// ErrorMessage returns the value at path in the JSON of v, or fallback when there is none. A path
// is dotted property names; a name ending in [] takes the first item of that array. A value that is
// no string is written as JSON.
func ErrorMessage(v any, path, fallback string) string {
	data, err := json.Marshal(v)
	var cur any
	if err == nil {
		err = json.Unmarshal(data, &cur)
	}
	if err != nil {
		return fallback
	}

	for _, seg := range strings.Split(path, ".") {
		name, isFirst := strings.CutSuffix(seg, "[]")
		obj, isObject := cur.(map[string]any)
		if !isObject {
			return fallback
		}
		cur = obj[name]
		if !isFirst {
			continue
		}
		items, isArray := cur.([]any)
		if !isArray || len(items) == 0 {
			return fallback
		}
		cur = items[0]
	}

	switch value := cur.(type) {
	case nil:
		return fallback
	case string:
		return value
	default:
		text, _ := json.Marshal(value) // decoded JSON always encodes again
		return string(text)
	}
}

// SetErrorMessage stores message at path in dst, a pointer, by decoding the JSON the path
// describes into it.
func SetErrorMessage(dst any, path, message string) error {
	var value any = message
	segs := strings.Split(path, ".")
	for i := len(segs) - 1; i >= 0; i-- {
		name, isFirst := strings.CutSuffix(segs[i], "[]")
		if isFirst {
			value = []any{value}
		}
		value = map[string]any{name: value}
	}

	data, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(data, dst)
	}
	return err
}
