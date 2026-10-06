// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package mcptool holds what generated MCP tools call: their input, result and error.
package mcptool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// toolBodyLimit is how many bytes of an error body a tool error carries, so a large page does
// not fill the assistant's context.
const toolBodyLimit = 4096

// Result is the structured content of an MCP tool. Clients before protocol 2026-07-28 take
// only a JSON object there, so Value is written as it is when it is one, else as {"result": Value}.
type Result struct {
	Value any
}

// MarshalJSON writes Value as a JSON object.
func (r Result) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(r.Value)
	if err != nil {
		return nil, err
	}
	if data[0] == '{' {
		return data, nil
	}
	return json.Marshal(map[string]json.RawMessage{"result": data})
}

// Input decodes the arguments of an MCP tool call into in again. The SDK reads them through
// float64, which rounds an integer above 2^53; an integer written as plain digits keeps all of
// them here. A key the call left out keeps what in holds, so the defaults the SDK filled in stay.
func Input(args json.RawMessage, in any) error {
	if len(args) == 0 {
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader(args))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}

	data, err := json.Marshal(floatNumbers(v))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, in)
}

// Error is err as an MCP tool reports it. The assistant reads only the message, so the
// message of a *runtime.APIError is followed by the response body, at most 4 KiB of it.
func Error(err error) error {
	var apiErr *runtime.APIError
	if !errors.As(err, &apiErr) {
		return err
	}

	body := bytes.TrimSpace(apiErr.Body)
	if len(body) == 0 {
		return err
	}
	return fmt.Errorf("%w\n%s", err, bodyText(body))
}

// bodyText is body as text: cut after toolBodyLimit bytes where a character starts, or only its
// size when it is no UTF-8 text.
func bodyText(body []byte) string {
	if !utf8.Valid(body) {
		return fmt.Sprintf("(%d bytes, not UTF-8 text)", len(body))
	}

	if len(body) <= toolBodyLimit {
		return string(body)
	}

	n := toolBodyLimit
	for !utf8.RuneStart(body[n]) {
		n--
	}
	return fmt.Sprintf("%s\n(%d more bytes left out)", body[:n], len(body)-n)
}

// floatNumbers reads each number of v written with a fraction or an exponent as a float64, as the
// SDK does, so 1.0 and 1e3 still decode into an integer type.
func floatNumbers(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for key, value := range v {
			v[key] = floatNumbers(value)
		}
	case []any:
		for i, value := range v {
			v[i] = floatNumbers(value)
		}
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") {
			f, _ := v.Float64() // out of range it is an infinity, which does not marshal
			return f
		}
	}
	return v
}
