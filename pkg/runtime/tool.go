// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// toolBodyLimit is how many bytes of an error body a tool error carries, so a large page does
// not fill the assistant's context.
const toolBodyLimit = 4096

// ToolResult is the structured content of an MCP tool. Clients before protocol 2026-07-28 take
// only a JSON object there, so Value is written as it is when it is one, else as {"result": Value}.
type ToolResult struct {
	Value any
}

// MarshalJSON writes Value as a JSON object.
func (r ToolResult) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(r.Value)
	if err != nil {
		return nil, err
	}
	if data[0] == '{' {
		return data, nil
	}
	return json.Marshal(map[string]json.RawMessage{"result": data})
}

// ToolError is err as an MCP tool reports it. The assistant reads only the message, so the
// message of an *APIError is followed by the response body, at most 4 KiB of it.
func ToolError(err error) error {
	var apiErr *APIError
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
