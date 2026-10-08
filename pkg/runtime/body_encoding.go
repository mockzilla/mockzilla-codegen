// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The encoding object of a form body: the content type or the style of each property.

package runtime

import (
	"fmt"
	"slices"
	"strings"
)

// PropertyEncoding is a property written in ContentType, or as a query parameter of Style when set.
type PropertyEncoding struct {
	ContentType string
	Style       Style
	IsExplode   bool
	IsReserved  bool
}

// Encoding is how a form body writes each property, by name, as its encoding object says.
type Encoding map[string]PropertyEncoding

// param is the query parameter a property of name is written as, and false when it has no style.
func (e Encoding) param(name string) (Param, bool) {
	pe := e[name]
	if pe.Style == "" {
		return Param{}, false
	}
	return Param{Name: name, Style: pe.Style, IsExplode: pe.IsExplode, IsReserved: pe.IsReserved}, true
}

// mediaTypes are the media types declared for name, in the order the spec lists them.
func (e Encoding) mediaTypes(name string) []string {
	var out []string
	for t := range strings.SplitSeq(e[name].ContentType, ",") {
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

// formPairs writes the JSON value v of name in its content type, else an object as JSON and a list item by item.
func (e Encoding) formPairs(name string, v any) ([]pair, error) {
	mediaType, err := e.partType(name, isFormText(v))
	if err != nil {
		return nil, err
	}
	key := escapeQuery(name, false)
	if IsJSON(mediaType) {
		return []pair{{name: key, value: escapeQuery(string(encodeJSON(v)), false)}}, nil
	}

	items, isList := v.([]any)
	if !isList {
		items = []any{v}
	}
	var out []pair
	for _, item := range items {
		switch item.(type) {
		case nil:
		case map[string]any, []any:
			out = append(out, pair{name: key, value: escapeQuery(string(encodeJSON(item)), false)})
		default:
			out = append(out, pair{name: key, value: escapeQuery(formText(item), false)})
		}
	}
	return out, nil
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
