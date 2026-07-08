// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"encoding/json"
	"reflect"
)

const (
	schemaDraft     = "https://json-schema.org/draft/2020-12/schema"
	byteSizePattern = `^[0-9]+(B|KB|MB|GB)?$`
	durationPattern = `^(0|([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+)$`
)

// Schema returns the JSON schema (draft 2020-12) of the config file, built from the config types.
// Keys are sorted, so the output is the same on every run.
func Schema() ([]byte, error) {
	root := typeSchema(reflect.TypeFor[Config]())
	root["$schema"] = schemaDraft
	root["title"] = "codegen config"

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(root)
	return buf.Bytes(), err
}

func typeSchema(t reflect.Type) map[string]any {
	switch t {
	case reflect.TypeFor[ByteSize]():
		return map[string]any{"type": []string{"integer", "string"}, "minimum": 0, "pattern": byteSizePattern}
	case reflect.TypeFor[Duration]():
		return map[string]any{"type": "string", "pattern": durationPattern}
	}

	switch t.Kind() {
	case reflect.Pointer:
		return typeSchema(t.Elem())
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": typeSchema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": typeSchema(t.Elem())}
	case reflect.Struct:
		return structSchema(t)
	default:
		return map[string]any{}
	}
}

func structSchema(t reflect.Type) map[string]any {
	props := make(map[string]any)
	for _, f := range fieldsOf(t) {
		s := typeSchema(f.typ)
		if f.desc != "" {
			s["description"] = f.desc
		}
		if f.enum != nil {
			s["enum"] = f.enum
		}
		props[f.key] = s
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": props}
}
