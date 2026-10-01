// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"reflect"
	"slices"
	"strconv"

	"go.yaml.in/yaml/v4"
)

const (
	nullTag  = "!!null"
	mapTag   = "!!map"
	mergeTag = "!!merge"
)

func walk(n *yaml.Node, t reflect.Type, path string) []string {
	if n.Kind == yaml.AliasNode {
		n = n.Alias
	}

	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	var unknown []string
	switch {
	case n.Kind == yaml.MappingNode && t.Kind() == reflect.Struct:
		unknown = walkStruct(n, t, path)
	case n.Kind == yaml.MappingNode && t.Kind() == reflect.Map:
		for i := 0; i < len(n.Content); i += 2 {
			if key := n.Content[i]; key.ShortTag() != mergeTag {
				unknown = append(unknown, walk(n.Content[i+1], t.Elem(), path+"."+key.Value)...)
			}
		}
	case n.Kind == yaml.SequenceNode && t.Kind() == reflect.Slice:
		for i, item := range n.Content {
			unknown = append(unknown, walk(item, t.Elem(), path+"["+strconv.Itoa(i)+"]")...)
		}
	}
	return unknown
}

func walkStruct(n *yaml.Node, t reflect.Type, path string) []string {
	fields := fieldsOf(t)

	var unknown []string
	for i := 0; i < len(n.Content); i += 2 {
		key, value := n.Content[i], n.Content[i+1]
		if key.ShortTag() == mergeTag {
			continue
		}

		keyPath := key.Value
		if path != "" {
			keyPath = path + "." + key.Value
		}

		at := slices.IndexFunc(fields, func(f field) bool { return f.key == key.Value })
		if at < 0 {
			unknown = append(unknown, keyPath)
			continue
		}

		typ := fields[at].typ
		// A block key with no value (`mcp:`) becomes an empty block, so the key alone turns it on.
		if value.ShortTag() == nullTag && typ.Kind() == reflect.Pointer && typ.Elem().Kind() == reflect.Struct {
			*value = yaml.Node{Kind: yaml.MappingNode, Tag: mapTag, Line: value.Line, Column: value.Column}
			continue
		}
		unknown = append(unknown, walk(value, typ, keyPath)...)
	}
	return unknown
}
