// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Writing oneOf and anyOf unions as JSON, with their discriminator.

package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime/validation"
)

// MarshalTagged is MarshalOneOf, or MarshalUnion for anyOf, that fills or checks the discriminator.
func MarshalTagged(shared any, u Union, variants ...any) ([]byte, error) {
	var set []any
	var picked []int
	isSet := make([]bool, len(variants))
	for i, v := range variants {
		if !isNil(v) {
			set = append(set, v)
			picked = append(picked, i)
			isSet[i] = true
		}
	}
	if !u.IsAnyOf {
		if err := validation.AtMostOne(isSet...); err != nil {
			return nil, err
		}
	}
	data, err := MarshalUnion(shared, set...)
	if err != nil || len(picked) == 0 || jsonKind(data) != KindObject {
		return data, err
	}
	return u.tag(data, picked)
}

// DiscriminatorError is the discriminator error of a MarshalTagged result, nil for any other.
func DiscriminatorError(_ []byte, err error) error {
	var wrapped *json.MarshalerError
	var e validation.Error
	if errors.As(err, &wrapped) || !errors.As(err, &e) || e.Rule != validation.RuleDiscriminator {
		return nil
	}
	return e
}

// MarshalOneOf is MarshalUnion for a oneOf, where more than one set variant is an error.
func MarshalOneOf(shared any, set ...any) ([]byte, error) {
	if err := validation.AtMostOne(slices.Repeat([]bool{true}, len(set))...); err != nil {
		return nil, err
	}
	return MarshalUnion(shared, set...)
}

// MarshalUnion writes the set variants of a union merged with shared, its own properties or nil.
// When a part is no object, the first set variant is written alone; nothing set writes null.
func MarshalUnion(shared any, set ...any) ([]byte, error) {
	values := set
	if shared != nil {
		values = append([]any{shared}, set...)
	}
	parts := make([][]byte, len(values))
	for i, v := range values {
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		parts[i] = data
	}

	switch len(parts) {
	case 0:
		return []byte("null"), nil
	case 1:
		return parts[0], nil
	}
	merged, err := mergeObjects(parts...)
	if errors.Is(err, ErrNotObject) && len(set) > 0 {
		return parts[len(parts)-len(set)], nil
	}
	return merged, err
}

// MarshalUnionText writes data, the JSON of a union of scalars, as text without string quotes.
func MarshalUnionText(data []byte, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}

	switch jsonKind(data) {
	case KindString:
		var s string
		err = json.Unmarshal(data, &s)
		return []byte(s), err
	case KindBool, KindInteger, KindNumber:
		return data, nil
	default:
	}
	return nil, fmt.Errorf("%w: cannot write %s as text", ErrParamValue, data)
}

// mergeObjects merges JSON objects into one. A key keeps the place it first appears at and takes
// the last value it has.
func mergeObjects(parts ...[]byte) ([]byte, error) {
	var keys []string
	values := map[string]json.RawMessage{}
	for _, p := range parts {
		dec := json.NewDecoder(bytes.NewReader(p))
		if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
			return nil, fmt.Errorf("%w: %.20q", ErrNotObject, p)
		}
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := tok.(string)
			var value json.RawMessage
			if err = dec.Decode(&value); err != nil {
				return nil, err
			}
			if _, isKnown := values[key]; !isKnown {
				keys = append(keys, key)
			}
			values[key] = value
		}
	}

	var b bytes.Buffer
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(key))
		b.WriteByte(':')
		b.Write(values[key])
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// isNil reports a nil pointer, slice, map or interface, or no value at all.
func isNil(v any) bool {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return rv.IsNil()
	case reflect.Invalid:
		return true
	default:
		return false
	}
}
