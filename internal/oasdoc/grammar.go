// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

type shape int

const (
	shapeOne shape = iota
	shapeMap
	shapeList
	shapeOneOrList
)

type field struct {
	kind  Kind
	shape shape
}

// Visit calls fn on n, an object of kind k at ptr, then on every object under it in document order.
// Only places the OpenAPI grammar gives a kind are visited: never example values or extensions.
// fn returning false skips the children of that node.
func Visit(n *yaml.Node, ptr string, k Kind, fn func(ptr string, n *yaml.Node, k Kind) bool) {
	if n.Kind != yaml.MappingNode || !fn(ptr, n, k) {
		return
	}

	for i := 0; i+1 < len(n.Content); i += 2 {
		key, value := n.Content[i].Value, n.Content[i+1]
		f, ok := fieldOf(k, key)
		if !ok {
			continue
		}

		at := ptr + "/" + Escape(key)
		switch {
		case f.shape == shapeMap && value.Kind == yaml.MappingNode:
			for j := 0; j+1 < len(value.Content); j += 2 {
				Visit(value.Content[j+1], at+"/"+Escape(value.Content[j].Value), f.kind, fn)
			}
		case (f.shape == shapeList || f.shape == shapeOneOrList) && value.Kind == yaml.SequenceNode:
			for j, item := range value.Content {
				Visit(item, at+"/"+strconv.Itoa(j), f.kind, fn)
			}
		case f.shape == shapeOne || f.shape == shapeOneOrList:
			Visit(value, at, f.kind, fn)
		}
	}
}

func fieldOf(k Kind, key string) (field, bool) {
	switch k {
	case KindDocument:
		return documentField(key)
	case KindComponents:
		return componentsField(key)
	case KindPaths, KindCallback:
		return field{kind: KindPathItem}, !strings.HasPrefix(key, "x-")
	case KindResponses:
		return field{kind: KindResponse}, !strings.HasPrefix(key, "x-")
	case KindPathItem:
		return pathItemField(key)
	case KindOperation:
		return operationField(key)
	case KindParameter, KindHeader:
		return parameterField(key)
	case KindRequestBody:
		return field{kind: KindMediaType, shape: shapeMap}, key == "content"
	case KindMediaType, KindEncoding:
		return mediaTypeField(k, key)
	case KindResponse:
		return responseField(key)
	case KindSchema:
		return schemaField(key)
	default:
		return field{}, false
	}
}

func documentField(key string) (field, bool) {
	switch key {
	case "paths":
		return field{kind: KindPaths}, true
	case "webhooks":
		return field{kind: KindPathItem, shape: shapeMap}, true
	case "components":
		return field{kind: KindComponents}, true
	default:
		return field{}, false
	}
}

func componentsField(key string) (field, bool) {
	k := SectionKind(key)
	return field{kind: k, shape: shapeMap}, k != KindNone
}

func pathItemField(key string) (field, bool) {
	switch key {
	case "get", "put", "post", "delete", "options", "head", "patch", "trace", "query":
		return field{kind: KindOperation}, true
	case "additionalOperations":
		return field{kind: KindOperation, shape: shapeMap}, true
	case "parameters":
		return field{kind: KindParameter, shape: shapeList}, true
	default:
		return field{}, false
	}
}

func operationField(key string) (field, bool) {
	switch key {
	case "parameters":
		return field{kind: KindParameter, shape: shapeList}, true
	case "requestBody":
		return field{kind: KindRequestBody}, true
	case "responses":
		return field{kind: KindResponses}, true
	case "callbacks":
		return field{kind: KindCallback, shape: shapeMap}, true
	default:
		return field{}, false
	}
}

func parameterField(key string) (field, bool) {
	switch key {
	case "schema":
		return field{kind: KindSchema}, true
	case "content":
		return field{kind: KindMediaType, shape: shapeMap}, true
	case "examples":
		return field{kind: KindExample, shape: shapeMap}, true
	default:
		return field{}, false
	}
}

// mediaTypeField covers encodings too: 3.2 nests encodings the way media types hold them.
func mediaTypeField(k Kind, key string) (field, bool) {
	switch key {
	case "schema", "itemSchema":
		return field{kind: KindSchema}, k == KindMediaType
	case "examples":
		return field{kind: KindExample, shape: shapeMap}, k == KindMediaType
	case "headers":
		return field{kind: KindHeader, shape: shapeMap}, k == KindEncoding
	case "encoding":
		return field{kind: KindEncoding, shape: shapeMap}, true
	case "prefixEncoding":
		return field{kind: KindEncoding, shape: shapeList}, true
	case "itemEncoding":
		return field{kind: KindEncoding}, true
	default:
		return field{}, false
	}
}

func responseField(key string) (field, bool) {
	switch key {
	case "headers":
		return field{kind: KindHeader, shape: shapeMap}, true
	case "content":
		return field{kind: KindMediaType, shape: shapeMap}, true
	case "links":
		return field{kind: KindLink, shape: shapeMap}, true
	default:
		return field{}, false
	}
}

func schemaField(key string) (field, bool) {
	switch key {
	case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas":
		return field{kind: KindSchema, shape: shapeMap}, true
	case "allOf", "anyOf", "oneOf", "prefixItems":
		return field{kind: KindSchema, shape: shapeList}, true
	case "items":
		return field{kind: KindSchema, shape: shapeOneOrList}, true
	case "additionalProperties", "not", "if", "then", "else", "contains", "propertyNames",
		"unevaluatedItems", "unevaluatedProperties", "additionalItems", "contentSchema":
		return field{kind: KindSchema}, true
	default:
		return field{}, false
	}
}
