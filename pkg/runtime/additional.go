// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"mime/multipart"
	"reflect"
	"slices"
)

// MarshalAdditional writes fields, a struct whose additional properties are tagged json:"-", as a
// JSON object, then adds the entries of extra in key order. Keys in known, the struct's own
// property names, are left out of extra.
func MarshalAdditional[T any](fields any, extra map[string]T, known ...string) ([]byte, error) {
	data, err := json.Marshal(fields)
	if err != nil || len(extra) == 0 {
		return data, err
	}

	var b bytes.Buffer
	b.Write(data[:len(data)-1])
	isFirst := len(data) == len("{}")
	for _, key := range slices.Sorted(maps.Keys(extra)) {
		if slices.Contains(known, key) {
			continue
		}
		var value []byte
		if value, err = json.Marshal(extra[key]); err != nil {
			return nil, fmt.Errorf("%w %q: %w", ErrAdditionalProperty, key, err)
		}
		if !isFirst {
			b.WriteByte(',')
		}
		isFirst = false
		name, _ := json.Marshal(key) // a string always marshals
		b.Write(name)
		b.WriteByte(':')
		b.Write(value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// UnmarshalAdditional decodes data into fields, then every member whose key is not in known into
// extra, keeping the entries extra already has.
func UnmarshalAdditional[T any](data []byte, fields any, extra *map[string]T, known ...string) error {
	if err := json.Unmarshal(data, fields); err != nil {
		return err
	}

	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(members)) {
		if slices.Contains(known, key) {
			continue
		}
		var value T
		if err := json.Unmarshal(members[key], &value); err != nil {
			return fmt.Errorf("%w %q: %w", ErrAdditionalProperty, key, err)
		}
		if *extra == nil {
			*extra = make(map[string]T)
		}
		(*extra)[key] = value
	}
	return nil
}

// UnmarshalAdditionalForm reads form into fields, then every name not in known into extra.
func UnmarshalAdditionalForm[T any](form *multipart.Form, fields any, extra *map[string]T, known ...string) error {
	if err := fillPointer(form, fields); err != nil {
		return err
	}

	nested := formTree(form.Value)
	for _, name := range slices.Sorted(maps.Keys(nested)) {
		if slices.Contains(known, name) {
			continue
		}
		var value T
		if err := (assigner{isLoose: true}).assign(reflect.ValueOf(&value).Elem(), nested[name]); err != nil {
			return fmt.Errorf("%w %q: %w", ErrAdditionalProperty, name, err)
		}
		if *extra == nil {
			*extra = make(map[string]T)
		}
		(*extra)[name] = value
	}
	return nil
}
