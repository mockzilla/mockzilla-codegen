// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Setting Go values from the text of params and forms, by reflection.

package runtime

import (
	"cmp"
	"encoding"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Loose values are strings that a form or a parameter carried, read into an untyped target.
var (
	looseInt   = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	looseFloat = regexp.MustCompile(`^-?(0|[1-9][0-9]*)\.[0-9]+$`)
)

var textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()

// assigner sets Go values from decoded text: a string, a list of strings, or a map of them, nested.
// With isLoose, text going into an untyped target becomes a bool or a number when it reads as one.
type assigner struct {
	isLoose bool
}

// assign stores v, a string, []string, []any, map[string]string or map[string]any, in dst, which
// must be settable.
func (a assigner) assign(dst reflect.Value, v any) error {
	if target, ok := targetOf(dst); ok {
		return a.assign(target, v)
	}
	switch dst.Kind() {
	case reflect.Pointer:
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return a.assign(dst.Elem(), v)
	case reflect.Interface:
		dst.Set(reflect.ValueOf(a.untyped(v)))
		return nil
	default:
	}
	if nested, isNested := v.(map[string]any); isNested && dst.CanAddr() {
		if u, ok := dst.Addr().Interface().(FormUnmarshaler); ok {
			return u.UnmarshalForm(&multipart.Form{Value: formValues(nested)})
		}
	}

	switch v := v.(type) {
	case string:
		return a.text(dst, v)
	case []string:
		return a.list(dst, v)
	case []any:
		return a.list(dst, v)
	case map[string]string:
		return a.object(dst, v)
	case map[string]any:
		return a.object(dst, v)
	}
	return nil
}

func (a assigner) text(dst reflect.Value, s string) error {
	if dst.CanAddr() && dst.Addr().Type().Implements(textUnmarshaler) {
		u, _ := dst.Addr().Interface().(encoding.TextUnmarshaler)
		return u.UnmarshalText([]byte(s))
	}
	if isBytes(dst.Type()) {
		data, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return fmt.Errorf("%w: %q is no base64", ErrParamValue, s)
		}
		dst.SetBytes(data)
		return nil
	}

	var err error
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(s)
	case reflect.Bool:
		var b bool
		if b, err = strconv.ParseBool(s); err == nil {
			dst.SetBool(b)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var n int64
		if n, err = strconv.ParseInt(s, 10, dst.Type().Bits()); err == nil {
			dst.SetInt(n)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var n uint64
		if n, err = strconv.ParseUint(s, 10, dst.Type().Bits()); err == nil {
			dst.SetUint(n)
		}
	case reflect.Float32, reflect.Float64:
		var f float64
		if f, err = strconv.ParseFloat(s, dst.Type().Bits()); err == nil {
			dst.SetFloat(f)
		}
	case reflect.Slice:
		return a.list(dst, []string{s})
	case reflect.Struct, reflect.Map:
		return a.jsonText(dst, s)
	case reflect.Invalid, reflect.Complex64, reflect.Complex128, reflect.Array, reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Pointer, reflect.UnsafePointer:
		return fmt.Errorf("%w: cannot decode text into %s", ErrParamValue, dst.Type())
	}
	if err != nil {
		return fmt.Errorf("%w: %q is no %s", ErrParamValue, s, dst.Type())
	}
	return nil
}

// json decodes s as JSON into a struct or map, which a union or an object of a parameter is.
func (a assigner) json(dst reflect.Value, s string) error {
	if err := json.Unmarshal([]byte(s), dst.Addr().Interface()); err != nil {
		return fmt.Errorf("%w: %w", ErrParamValue, err)
	}
	return nil
}

// jsonText decodes s as JSON, else as a string, so a union takes plain text for a string variant.
func (a assigner) jsonText(dst reflect.Value, s string) error {
	quoted, _ := json.Marshal(s)
	if !json.Valid([]byte(s)) {
		return a.json(dst, string(quoted))
	}
	err := a.json(dst, s)
	if err != nil && isLiteral([]byte(s)) && a.json(dst, string(quoted)) == nil {
		return nil
	}
	return err
}

func (a assigner) list(dst reflect.Value, items any) error {
	values := reflect.ValueOf(items)
	n := values.Len()
	if dst.Kind() == reflect.Slice && !isBytes(dst.Type()) {
		out := reflect.MakeSlice(dst.Type(), n, n)
		for i := range n {
			if err := a.assign(out.Index(i), values.Index(i).Interface()); err != nil {
				return err
			}
		}
		dst.Set(out)
		return nil
	}
	if n == 0 {
		return nil
	}
	return a.assign(dst, values.Index(0).Interface())
}

func (a assigner) object(dst reflect.Value, fields any) error {
	m := reflect.ValueOf(fields)
	switch dst.Kind() {
	case reflect.Struct:
		for i := range dst.NumField() {
			f := dst.Type().Field(i)
			name := jsonName(f)
			if name == "" || !f.IsExported() {
				continue
			}
			if v := m.MapIndex(reflect.ValueOf(name)); v.IsValid() {
				if err := a.assign(dst.Field(i), v.Interface()); err != nil {
					return err
				}
			}
		}
		return nil
	case reflect.Map:
		if dst.IsNil() {
			dst.Set(reflect.MakeMap(dst.Type()))
		}
		for _, key := range sortedMapKeys(m) {
			item := reflect.New(dst.Type().Elem()).Elem()
			if err := a.assign(item, m.MapIndex(key).Interface()); err != nil {
				return err
			}
			dst.SetMapIndex(key.Convert(dst.Type().Key()), item)
		}
		return nil
	default:
	}
	return fmt.Errorf("%w: cannot decode an object into %s", ErrParamValue, dst.Type())
}

// untyped is v for an untyped target: lists become []any, objects map[string]any, and text a
// bool or a number when it reads as one and the assigner is loose.
func (a assigner) untyped(v any) any {
	switch v := v.(type) {
	case string:
		return a.loose(v)
	case []string:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = a.loose(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = a.untyped(item)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = a.loose(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = a.untyped(item)
		}
		return out
	}
	return v
}

// loose reads s as a bool or a number when it is written as one, with no leading zeros, sign
// character or exponent, and keeps it a string otherwise.
func (a assigner) loose(s string) any {
	if !a.isLoose {
		return s
	}
	switch {
	case s == "true" || s == "false":
		return s == "true"
	case looseInt.MatchString(s):
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
	case looseFloat.MatchString(s):
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return s
}

// jsonName is the name a field has in JSON: its json tag, else its Go name; "-" ignores it.
func jsonName(f reflect.StructField) string {
	tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	switch tag {
	case "":
		return f.Name
	case "-":
		return ""
	}
	return tag
}

func sortedMapKeys(m reflect.Value) []reflect.Value {
	keys := m.MapKeys()
	slices.SortFunc(keys, func(a, b reflect.Value) int { return cmp.Compare(a.String(), b.String()) })
	return keys
}
