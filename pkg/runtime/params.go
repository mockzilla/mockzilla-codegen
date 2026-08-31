// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// Style is how a parameter is written, as OpenAPI names them.
type Style string

const (
	StyleSimple         Style = "simple"
	StyleLabel          Style = "label"
	StyleMatrix         Style = "matrix"
	StyleForm           Style = "form"
	StyleSpaceDelimited Style = "spaceDelimited"
	StylePipeDelimited  Style = "pipeDelimited"
	StyleDeepObject     Style = "deepObject"
)

// shape is what a parameter's Go type holds: one value, a list, or an object with named values.
type shape int

const (
	shapeValue shape = iota
	shapeList
	shapeObject
)

// separators join the items of a list that is written in one value, by style.
var separators = map[Style]string{StyleSpaceDelimited: " ", StylePipeDelimited: "|"}

var textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()

// Param describes one parameter: its name, how it is written, and whether it must be there. IsJSON
// is set for a parameter with content application/json.
type Param struct {
	Name       string
	Style      Style
	IsExplode  bool
	IsRequired bool
	IsJSON     bool
}

// pair is one named value of an object parameter, in the order it is written.
type pair struct {
	name  string
	value string
}

// DecodePath decodes a path segment into dst, a pointer to the parameter's type.
func DecodePath(raw string, p Param, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	if p.IsJSON {
		return assigner{}.json(target, raw)
	}

	sh := shapeOf(target.Type())
	switch p.Style {
	case StyleLabel:
		raw = strings.TrimPrefix(raw, ".")
		if p.IsExplode {
			return assigner{}.assign(target, pieces(raw, ".", sh, true))
		}
	case StyleMatrix:
		raw = strings.TrimPrefix(raw, ";")
		if p.IsExplode && sh != shapeValue {
			items := strings.Split(raw, ";")
			for i, item := range items {
				items[i] = strings.TrimPrefix(item, p.Name+"=")
			}
			return assigner{}.assign(target, tree(items, sh, sh == shapeObject))
		}
		raw = strings.TrimPrefix(raw, p.Name+"=")
	case StyleSimple, StyleForm, StyleSpaceDelimited, StylePipeDelimited, StyleDeepObject:
	}
	return assigner{}.assign(target, pieces(raw, ",", sh, p.IsExplode))
}

// DecodeQuery decodes a query parameter into dst, a pointer to the parameter's type. A parameter
// that is not there leaves dst as it is, unless it is required.
func DecodeQuery(q url.Values, p Param, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	sh := shapeOf(target.Type())

	switch {
	case p.Style == StyleDeepObject:
		fields := nested(q, p.Name)
		if fields == nil {
			return absent(p)
		}
		return assigner{}.assign(target, fields)
	case sh == shapeObject && p.IsExplode && !p.IsJSON:
		if len(q) == 0 {
			return absent(p)
		}
		return assigner{}.assign(target, firstValues(q))
	}

	values, ok := q[p.Name]
	if !ok {
		return absent(p)
	}
	return decodeValues(target, values, p, sh)
}

// DecodeHeader decodes a header into dst, a pointer to the parameter's type.
func DecodeHeader(h http.Header, p Param, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	values := h.Values(p.Name)
	if len(values) == 0 {
		return absent(p)
	}
	raw := strings.Join(values, ",")
	if p.IsJSON {
		return assigner{}.json(target, raw)
	}
	return assigner{}.assign(target, pieces(raw, ",", shapeOf(target.Type()), p.IsExplode))
}

// DecodeCookie decodes a cookie into dst, a pointer to the parameter's type.
func DecodeCookie(cookies []*http.Cookie, p Param, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	sh := shapeOf(target.Type())
	q := url.Values{}
	for _, c := range cookies {
		q.Add(c.Name, c.Value)
	}

	if sh == shapeObject && p.IsExplode && !p.IsJSON {
		return assigner{}.assign(target, firstValues(q))
	}
	values, ok := q[p.Name]
	if !ok {
		return absent(p)
	}
	return decodeValues(target, values, p, sh)
}

// EncodePath writes v as a path segment.
func EncodePath(v any, p Param) (string, error) {
	t, err := encodeTree(v, p)
	if err != nil {
		return "", err
	}

	switch p.Style {
	case StyleLabel:
		if p.IsExplode {
			return "." + join(t, ".", true), nil
		}
		return "." + join(t, ",", false), nil
	case StyleMatrix:
		switch t := t.(type) {
		case []string:
			if p.IsExplode {
				return ";" + p.Name + "=" + strings.Join(t, ";"+p.Name+"="), nil
			}
		case []pair:
			if p.IsExplode {
				return ";" + join(t, ";", true), nil
			}
		}
		return ";" + p.Name + "=" + join(t, ",", false), nil
	case StyleSimple, StyleForm, StyleSpaceDelimited, StylePipeDelimited, StyleDeepObject:
	}
	return join(t, ",", p.IsExplode), nil
}

// EncodeQuery adds v to q as the parameter p.
func EncodeQuery(v any, p Param, q url.Values) error {
	t, err := encodeTree(v, p)
	if err != nil || t == nil {
		return err
	}

	switch t := t.(type) {
	case []pair:
		switch {
		case p.Style == StyleDeepObject:
			for _, f := range t {
				q.Add(p.Name+"["+f.name+"]", f.value)
			}
		case p.IsExplode:
			for _, f := range t {
				q.Add(f.name, f.value)
			}
		default:
			q.Add(p.Name, join(t, separator(p.Style), false))
		}
	case []string:
		if p.IsExplode {
			for _, item := range t {
				q.Add(p.Name, item)
			}
			return nil
		}
		q.Add(p.Name, strings.Join(t, separator(p.Style)))
	case string:
		q.Add(p.Name, t)
	}
	return nil
}

// EncodeHeader writes v as a header value.
func EncodeHeader(v any, p Param) (string, error) {
	t, err := encodeTree(v, p)
	if err != nil {
		return "", err
	}
	return join(t, ",", p.IsExplode), nil
}

// EncodeCookie writes v as cookies: one, or one per item of an exploded list or object.
func EncodeCookie(v any, p Param) ([]*http.Cookie, error) {
	t, err := encodeTree(v, p)
	if err != nil || t == nil {
		return nil, err
	}

	var out []*http.Cookie
	switch t := t.(type) {
	case []string:
		if !p.IsExplode {
			return []*http.Cookie{{Name: p.Name, Value: strings.Join(t, ",")}}, nil
		}
		for _, item := range t {
			out = append(out, &http.Cookie{Name: p.Name, Value: item})
		}
	case []pair:
		if !p.IsExplode {
			return []*http.Cookie{{Name: p.Name, Value: join(t, ",", false)}}, nil
		}
		for _, f := range t {
			out = append(out, &http.Cookie{Name: f.name, Value: f.value})
		}
	case string:
		out = append(out, &http.Cookie{Name: p.Name, Value: t})
	}
	return out, nil
}

// Headers adds the fields of v, a struct of typed headers, to h and returns it, made when nil. A
// field that is nil or that cannot be written is left out.
func Headers(h http.Header, v any) http.Header {
	if h == nil {
		h = http.Header{}
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return h
	}

	for i := range rv.NumField() {
		f := rv.Type().Field(i)
		name := jsonName(f)
		if name == "" || !f.IsExported() {
			continue
		}
		if t, err := encodeTree(rv.Field(i).Interface(), Param{Style: StyleSimple}); err == nil && t != nil {
			h.Set(name, join(t, ",", false))
		}
	}
	return h
}

// decodeValues decodes the values a query or cookie parameter came with, one per item when
// exploded, else one holding every item.
func decodeValues(target reflect.Value, values []string, p Param, sh shape) error {
	switch {
	case p.IsJSON:
		return assigner{}.json(target, values[0])
	case sh == shapeValue:
		return assigner{}.assign(target, values[0])
	case p.IsExplode && sh == shapeList:
		return assigner{}.assign(target, values)
	}

	return assigner{}.assign(target, tree(strings.Split(values[0], separator(p.Style)), sh, false))
}

// separator is what a query style puts between the items of a list written in one value.
func separator(s Style) string {
	if sep, ok := separators[s]; ok {
		return sep
	}
	return ","
}

// pieces splits raw at sep as the shape of the target needs, and reads the pieces.
func pieces(raw, sep string, sh shape, isExplode bool) any {
	if sh == shapeValue {
		return raw
	}
	return tree(strings.Split(raw, sep), sh, isExplode)
}

// tree turns the pieces of a parameter into what the target holds: a list, or an object written
// as name=value pieces when exploded, else as alternating names and values.
func tree(items []string, sh shape, isExplode bool) any {
	if sh != shapeObject {
		return items
	}
	fields := map[string]string{}
	if isExplode {
		for _, item := range items {
			name, value, _ := strings.Cut(item, "=")
			fields[name] = value
		}
		return fields
	}
	for i := 0; i+1 < len(items); i += 2 {
		fields[items[i]] = items[i+1]
	}
	return fields
}

// nested reads the deepObject keys of name: name[a]=1, name[b][c]=2, into a map.
func nested(q url.Values, name string) map[string]any {
	var out map[string]any
	for key, values := range q {
		rest, ok := strings.CutPrefix(key, name+"[")
		if !ok || len(values) == 0 {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		setPath(out, splitBrackets("["+rest), values)
	}
	return out
}

// splitBrackets turns a[b][c] and [b][c] into its names; an empty pair of brackets is "".
func splitBrackets(key string) []string {
	head, rest, _ := strings.Cut(key, "[")
	var out []string
	if head != "" {
		out = append(out, head)
	}
	for rest != "" {
		name, tail, _ := strings.Cut(rest, "]")
		out = append(out, name)
		rest = strings.TrimPrefix(tail, "[")
	}
	return out
}

// setPath stores values at path in m: a list under the last name, or its first value alone.
func setPath(m map[string]any, path []string, values []string) {
	for _, name := range path[:len(path)-1] {
		child, ok := m[name].(map[string]any)
		if !ok {
			child = map[string]any{}
			m[name] = child
		}
		m = child
	}
	last := path[len(path)-1]
	if len(values) == 1 {
		m[last] = values[0]
		return
	}
	m[last] = values
}

func firstValues(q url.Values) map[string]string {
	out := make(map[string]string, len(q))
	for key, values := range q {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

func absent(p Param) error {
	if p.IsRequired {
		return fmt.Errorf("%w: %s", ErrParamMissing, p.Name)
	}
	return nil
}

// pointer returns what dst points to, ready to be set.
func pointer(dst any) (reflect.Value, error) {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return reflect.Value{}, fmt.Errorf("%w: the target must be a pointer", ErrParamValue)
	}
	return v.Elem(), nil
}

// shapeOf is what a Go type holds as a parameter. A type that reads text on its own, such as
// time.Time, is one value.
func shapeOf(t reflect.Type) shape {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case reflect.PointerTo(t).Implements(textUnmarshaler):
		return shapeValue
	case t.Kind() == reflect.Slice && t.Elem().Kind() != reflect.Uint8:
		return shapeList
	case t.Kind() == reflect.Struct || t.Kind() == reflect.Map:
		return shapeObject
	}
	return shapeValue
}

// encodeTree writes v as text: one string, a list of strings, or name-value pairs. A nil pointer
// gives nil.
func encodeTree(v any, p Param) (any, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, nil
		}
		rv = rv.Elem()
	}
	if p.IsJSON {
		data, err := json.Marshal(rv.Interface())
		return string(data), err
	}

	switch shapeOf(rv.Type()) {
	case shapeList:
		out := make([]string, rv.Len())
		for i := range rv.Len() {
			s, err := text(rv.Index(i))
			if err != nil {
				return nil, err
			}
			out[i] = s
		}
		return out, nil
	case shapeObject:
		return pairs(rv)
	default:
		return text(rv)
	}
}

func pairs(rv reflect.Value) ([]pair, error) {
	var out []pair
	add := func(name string, v reflect.Value) error {
		for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		}
		s, err := text(v)
		if err != nil {
			return err
		}
		out = append(out, pair{name: name, value: s})
		return nil
	}

	if rv.Kind() == reflect.Map {
		for _, key := range sortedMapKeys(rv) {
			if err := add(key.String(), rv.MapIndex(key)); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	for i := range rv.NumField() {
		f := rv.Type().Field(i)
		if name := jsonName(f); name != "" && f.IsExported() {
			if err := add(name, rv.Field(i)); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// text writes one value as a parameter carries it.
func text(v reflect.Value) (string, error) {
	if v.Type().Implements(textMarshaler) {
		m, _ := v.Interface().(encoding.TextMarshaler)
		b, err := m.MarshalText()
		return string(b), err
	}

	switch v.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, v.Type().Bits()), nil
	case reflect.Invalid, reflect.Complex64, reflect.Complex128, reflect.Array, reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.Struct, reflect.UnsafePointer:
	}
	return "", fmt.Errorf("%w: cannot write %s as a parameter", ErrParamValue, v.Type())
}

// join writes a tree as one string: items with sep between them, and pairs as name=value when
// exploded, else as alternating names and values.
func join(t any, sep string, isExplode bool) string {
	switch t := t.(type) {
	case string:
		return t
	case []string:
		return strings.Join(t, sep)
	case []pair:
		items := make([]string, 0, 2*len(t))
		for _, f := range t {
			if isExplode {
				items = append(items, f.name+"="+f.value)
				continue
			}
			items = append(items, f.name, f.value)
		}
		return strings.Join(items, sep)
	}
	return ""
}
