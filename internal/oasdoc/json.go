// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"bytes"
	"encoding/json"
)

// UnescapeSlashes turns \/ in a JSON spec into /: YAML 1.2 allows it, the YAML parser does not.
func UnescapeSlashes(data []byte) []byte {
	if !bytes.Contains(data, []byte(`\/`)) || !json.Valid(data) {
		return data
	}

	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '\\' {
			i++
			if data[i] != '/' {
				out = append(out, '\\')
			}
		}
		out = append(out, data[i])
	}
	return out
}
