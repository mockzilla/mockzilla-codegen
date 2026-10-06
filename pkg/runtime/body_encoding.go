// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The content types the encoding object of a form body declares for its properties.

package runtime

import (
	"fmt"
	"slices"
	"strings"
)

// Encoding is the content type the encoding of a form body declares for each property, by name.
type Encoding map[string]string

// mediaTypes are the media types declared for name, in the order the spec lists them.
func (e Encoding) mediaTypes(name string) []string {
	var out []string
	for t := range strings.SplitSeq(e[name], ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// isJSON reports a property whose declared media types are all JSON.
func (e Encoding) isJSON(name string) bool {
	types := e.mediaTypes(name)
	return len(types) > 0 && !slices.ContainsFunc(types, func(t string) bool { return !IsJSON(t) })
}

// partType is the first declared media type a value of name fits, JSON any value and others text.
func (e Encoding) partType(name string, isText bool) (string, error) {
	types := e.mediaTypes(name)
	if len(types) == 0 {
		return "", nil
	}
	for _, t := range types {
		if !strings.Contains(t, "*") && (isText || IsJSON(t)) {
			return t, nil
		}
	}
	return "", ContentTypeError(name + " in " + strings.Join(types, ", "))
}

// fileType is the media type of a file part: its own, else the one declared, else octet-stream.
func (e Encoding) fileType(name string, f File) (string, error) {
	if t := f.ContentType(); t != "" {
		return t, nil
	}
	types := e.mediaTypes(name)
	switch {
	case len(types) == 0:
		return "application/octet-stream", nil
	case len(types) == 1 && !strings.Contains(types[0], "*"):
		return types[0], nil
	}
	return "", fmt.Errorf("%w: %s: the file has no content type; the spec takes %s", ErrBodyValue, name, strings.Join(types, ", "))
}
