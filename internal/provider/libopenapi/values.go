// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func values(nodes []*yaml.Node) []spec.Value {
	if len(nodes) == 0 {
		return nil
	}

	out := make([]spec.Value, len(nodes))
	for i, n := range nodes {
		out[i] = value(n)
	}
	return out
}

func optionalValue(n *yaml.Node) *spec.Value {
	if n == nil {
		return nil
	}
	return new(value(n))
}

// value turns YAML-only numbers like 0x1F into JSON numbers; .inf and .nan stay strings.
func value(n *yaml.Node) spec.Value {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}

	switch n.Kind {
	case yaml.SequenceNode:
		return spec.Value{Kind: spec.KindArray, Items: values(n.Content)}
	case yaml.MappingNode:
		out := spec.Value{Kind: spec.KindObject, Fields: make([]spec.Field, 0, len(n.Content)/2)}
		for i := 0; i+1 < len(n.Content); i += 2 {
			out.Fields = append(out.Fields, spec.Field{Name: n.Content[i].Value, Value: value(n.Content[i+1])})
		}
		return out
	default:
		return scalar(n)
	}
}

func scalar(n *yaml.Node) spec.Value {
	switch n.ShortTag() {
	case "!!null":
		return spec.Value{Kind: spec.KindNull}
	case "!!bool":
		return spec.Value{Kind: spec.KindBool, Bool: strings.EqualFold(n.Value, "true")}
	case "!!int":
		if num, ok := intNumber(n.Value); ok {
			return spec.Value{Kind: spec.KindNumber, Num: num}
		}
	case "!!float":
		if num, ok := floatNumber(n.Value); ok {
			return spec.Value{Kind: spec.KindNumber, Num: num}
		}
	}
	return spec.Value{Kind: spec.KindString, Str: n.Value}
}

func intNumber(s string) (json.Number, bool) {
	if isJSONNumber(s) {
		return json.Number(s), true
	}
	plain := strings.ReplaceAll(s, "_", "")
	if i, err := strconv.ParseInt(plain, 0, 64); err == nil {
		return json.Number(strconv.FormatInt(i, 10)), true
	}
	if u, err := strconv.ParseUint(strings.TrimPrefix(plain, "+"), 0, 64); err == nil {
		return json.Number(strconv.FormatUint(u, 10)), true
	}
	return "", false
}

func floatNumber(s string) (json.Number, bool) {
	if isJSONNumber(s) {
		return json.Number(s), true
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, "_", ""), 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return "", false
	}
	return json.Number(strconv.FormatFloat(f, 'g', -1, 64)), true
}

func isJSONNumber(s string) bool {
	return s != "" && (s[0] == '-' || (s[0] >= '0' && s[0] <= '9')) && json.Valid([]byte(s))
}
