// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// JSON pointers: escaped tokens, and the index of a token in a mapping or a list.

package oasdoc

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

const strTag = "!!str"

// Escape encodes one reference token: ~ becomes ~0 and / becomes ~1.
func Escape(token string) string {
	if !strings.ContainsAny(token, "~/") {
		return token
	}
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

func Unescape(token string) string {
	if !strings.Contains(token, "~") {
		return token
	}
	return strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
}

// Split turns a pointer into unescaped tokens; a # prefix marks a $ref fragment, also percent-decoded.
func Split(ptr string) ([]string, error) {
	isFragment := strings.HasPrefix(ptr, "#")
	raw := strings.TrimPrefix(ptr, "#")
	if raw == "" {
		return nil, nil
	}
	if raw[0] != '/' {
		return nil, fmt.Errorf("%w: %q", ErrPointer, ptr)
	}

	tokens := strings.Split(raw[1:], "/")
	for i, t := range tokens {
		if isFragment {
			decoded, err := url.PathUnescape(t)
			if err != nil {
				return nil, fmt.Errorf("%w: %q: %w", ErrPointer, ptr, err)
			}
			t = decoded
		}
		tokens[i] = Unescape(t)
	}
	return tokens, nil
}

func keyIndex(n *yaml.Node, key string) int {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func seqIndex(n *yaml.Node, token string) (int, bool) {
	i, err := strconv.Atoi(token)
	if err != nil || i >= len(n.Content) || token != strconv.Itoa(i) || i < 0 {
		return 0, false
	}
	return i, true
}
