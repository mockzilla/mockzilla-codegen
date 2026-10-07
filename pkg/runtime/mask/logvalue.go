// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The slog value of a model with sensitive fields, from its masked JSON.

package mask

import (
	"bytes"
	"encoding/json"
	"log/slog"
)

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
