// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// YAML nodes as JSON values of the IR, with numbers kept as written.

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

// value reads scalars by the YAML 1.2 core schema: 0x1F is a number, 1_000 and .inf are strings.
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

// intNumber reads YAML 1.2 ints; the parser also tags YAML 1.1 forms such as 0b1 and 1_0.
func intNumber(s string) (json.Number, bool) {
	if isJSONNumber(s) {
		return json.Number(s), true
	}
	if digits, ok := strings.CutPrefix(s, "0o"); ok {
		return uintNumber(digits, 8)
	}
	if digits, ok := strings.CutPrefix(s, "0x"); ok {
		return uintNumber(digits, 16)
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return json.Number(strconv.FormatInt(i, 10)), true
	}
	return "", false
}

func uintNumber(digits string, base int) (json.Number, bool) {
	u, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return "", false
	}
	return json.Number(strconv.FormatUint(u, 10)), true
}

func floatNumber(s string) (json.Number, bool) {
	if isJSONNumber(s) {
		return json.Number(s), true
	}
	if strings.Contains(s, "_") {
		return "", false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return "", false
	}
	return json.Number(strconv.FormatFloat(f, 'g', -1, 64)), true
}

func isJSONNumber(s string) bool {
	return s != "" && (s[0] == '-' || (s[0] >= '0' && s[0] <= '9')) && json.Valid([]byte(s))
}
