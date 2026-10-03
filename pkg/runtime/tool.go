// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import "encoding/json"

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
