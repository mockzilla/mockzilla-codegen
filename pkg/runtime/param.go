// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Decoding and writing path, query, header and cookie parameters by their style.

package runtime

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"slices"
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

const (
	upperHex = "0123456789ABCDEF"
	// queryReserved are the reserved characters of RFC 3986 a query holds: all but [ ] and #.
	queryReserved = "!$&'()*+,;=:@/?"
)

// separators join the items of a list that is written in one value, by style.
var separators = map[Style]string{StyleSpaceDelimited: " ", StylePipeDelimited: "|"}

var (
	textMarshaler   = reflect.TypeFor[encoding.TextMarshaler]()
	jsonMarshaler   = reflect.TypeFor[json.Marshaler]()
	formUnmarshaler = reflect.TypeFor[FormUnmarshaler]()
)

// Param describes one parameter: its name, how it is written, and whether it must be there. IsJSON
// is set for content application/json; Default is the JSON a decoder sets when it is not there.
// IsReserved is allowReserved: a query value keeps the reserved characters a query holds.
type Param struct {
	Name       string
	Style      Style
	IsExplode  bool
	IsRequired bool
	IsJSON     bool
	IsReserved bool
	Default    string
}

// Query is the query of a request by key: each key unescaped, each value still percent-encoded,
// so a list is split at its commas before its items are unescaped.
type Query map[string][]string

// pair is one named value of an object parameter, in the order it is written.
type pair struct {
	name  string
	value string
}

// UnescapePath unescapes v, a path value a router cut from r.URL.RawPath when the request has one.
func UnescapePath(r *http.Request, v string) string {
	if r.URL.RawPath == "" {
		return v
	}
	if out, err := url.PathUnescape(v); err == nil {
		return out
	}
	return v
}

// DecodePath decodes a path segment into dst, a pointer to the parameter's type.
func DecodePath(raw string, p Param, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	if p.IsJSON {
		return setParamJSON(target, raw)
	}

	sh := shapeOf(target.Type())
	switch p.Style {
	case StyleLabel:
		raw = strings.TrimPrefix(raw, ".")
		if p.IsExplode {
			return setParam(target, pieces(raw, ".", sh, true))
		}
	case StyleMatrix:
		raw = strings.TrimPrefix(raw, ";")
		if p.IsExplode && sh != shapeValue {
			items := strings.Split(raw, ";")
			for i, item := range items {
				items[i] = strings.TrimPrefix(item, p.Name+"=")
			}
			return setParam(target, tree(items, sh, sh == shapeObject))
		}
		raw = strings.TrimPrefix(raw, p.Name+"=")
	case StyleSimple, StyleForm, StyleSpaceDelimited, StylePipeDelimited, StyleDeepObject:
	}
	return setParam(target, pieces(raw, ",", sh, p.IsExplode))
}

// ParseQuery splits raw, the query of a request, at & into its keys. A key that does not unescape
// is left out; a value is unescaped when its parameter is decoded.
func ParseQuery(raw string) Query {
	q := Query{}
	for item := range strings.SplitSeq(raw, "&") {
		if item == "" {
			continue
		}
		key, value, _ := strings.Cut(item, "=")
		if name, err := url.QueryUnescape(key); err == nil {
			q[name] = append(q[name], value)
		}
	}
	return q
}

// DecodeQuery decodes a query parameter into dst, a pointer to the parameter's type. A parameter
// that is not there sets its default, else leaves dst as it is, unless it is required. A value
// that does not unescape is ErrParamValue.
func DecodeQuery(q Query, p Param, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	sh := shapeOf(target.Type())

	switch {
	case p.Style == StyleDeepObject:
		var fields map[string]any
		if fields, err = nested(q, p.Name); err != nil {
			return err
		}
		if fields == nil {
			return missing(p, target)
		}
		return setParam(target, fields)
	case sh == shapeObject && p.IsExplode && !p.IsJSON:
		if len(q) == 0 {
			return missing(p, target)
		}
		var fields map[string]string
		if fields, err = firstValues(q, unescapeQuery); err != nil {
			return err
		}
		return setParam(target, fields)
	}

	values, ok := q[p.Name]
	if !ok {
		return missing(p, target)
	}
	return decodeValues(target, values, p, unescapeQuery)
}

// DecodeHeader decodes a header into dst, a pointer to the parameter's type.
func DecodeHeader(h http.Header, p Param, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	values := h.Values(p.Name)
	if len(values) == 0 {
		return missing(p, target)
	}
	raw := strings.Join(values, ",")
	if p.IsJSON {
		return setParamJSON(target, raw)
	}
	return setParam(target, pieces(raw, ",", shapeOf(target.Type()), p.IsExplode))
}

// DecodeCookie decodes a cookie into dst, a pointer to the parameter's type.
func DecodeCookie(cookies []*http.Cookie, p Param, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	q := url.Values{}
	for _, c := range cookies {
		q.Add(c.Name, c.Value)
	}

	if shapeOf(target.Type()) == shapeObject && p.IsExplode && !p.IsJSON {
		fields, _ := firstValues(q, keepValue) // keepValue never fails
		return setParam(target, fields)
	}
	values, ok := q[p.Name]
	if !ok {
		return missing(p, target)
	}
	return decodeValues(target, values, p, keepValue)
}

// DecodeQueryString decodes raw, the whole query, into dst: percent-encoded JSON or a form.
func DecodeQueryString(raw string, p Param, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	if raw == "" {
		return missing(p, target)
	}

	if p.IsJSON {
		if raw, err = url.PathUnescape(raw); err != nil {
			return fmt.Errorf("%w: %w", ErrParamValue, err)
		}
		return setParamJSON(target, raw)
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrParamValue, err)
	}
	return invalid(ErrParamValue, fillForm(target, &multipart.Form{Value: values}, nil))
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

// DecodeHeaders reads h into dst, a pointer to a struct of typed headers whose fields name their
// header in a json tag, each as a simple-style parameter. A header that is not there leaves its
// field as it is.
func DecodeHeaders(h http.Header, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	target = allocate(target)
	if target.Kind() != reflect.Struct {
		return fmt.Errorf("%w: typed headers need a struct, not %s", ErrParamValue, target.Type())
	}

	for i := range target.NumField() {
		f := target.Type().Field(i)
		name := jsonName(f)
		if name == "" || !f.IsExported() {
			continue
		}
		if err = DecodeHeader(h, Param{Name: name, Style: StyleSimple}, target.Field(i).Addr().Interface()); err != nil {
			return fmt.Errorf("%w %s: %w", ErrHeaderValue, name, err)
		}
	}
	return nil
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

// EncodeQuery writes v as the query parameter p, name=value pairs joined by &, and "" for a nil
// value. It percent-encodes as RFC 6570 expands a form-style query: a separator goes as it is and
// the same byte inside a name or value escaped, a space as %20. With IsReserved, the reserved
// characters a query holds and %XX escapes stay as they are.
func EncodeQuery(v any, p Param) (string, error) {
	t, err := encodeTree(v, p)
	if err != nil {
		return "", err
	}

	name, escaped := escapeQuery(p.Name, false), escapeTree(t, p.IsReserved)
	switch e := escaped.(type) {
	case nil:
		return "", nil
	case []pair:
		if p.IsExplode || p.Style == StyleDeepObject {
			return join(e, "&", true), nil
		}
	case []string:
		if p.IsExplode {
			named := make([]pair, len(e))
			for i, item := range e {
				named[i] = pair{name: name, value: item}
			}
			return join(named, "&", true), nil
		}
	}
	// A comma is reserved and goes as it is; a space or a pipe is escaped.
	return name + "=" + join(escaped, escapeQuery(separator(p.Style), true), false), nil
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

// decodeValues decodes the values a query or cookie parameter came with, one per item when
// exploded, else one holding every item. read turns a value as it came into its text.
func decodeValues(target reflect.Value, values []string, p Param, read func(string) (string, error)) error {
	sh := shapeOf(target.Type())
	if p.IsJSON || sh == shapeValue {
		raw, err := read(values[0])
		if err != nil {
			return err
		}
		if p.IsJSON {
			return setParamJSON(target, raw)
		}
		return setParam(target, raw)
	}
	if p.IsExplode && sh == shapeList {
		items, err := readAll(values, read)
		if err != nil {
			return err
		}
		return setParam(target, items)
	}

	// A comma inside an item comes escaped and one between items does not, so the value is split
	// before it is read. A space or a pipe comes escaped either way.
	sep := separator(p.Style)
	if sep == "," {
		items, err := readAll(strings.Split(values[0], sep), read)
		if err != nil {
			return err
		}
		return setParam(target, tree(items, sh, false))
	}
	raw, err := read(values[0])
	if err != nil {
		return err
	}
	return setParam(target, tree(strings.Split(raw, sep), sh, false))
}

// unescapeQuery reads a query value, with a + as a space, as WHATWG reads a form.
func unescapeQuery(s string) (string, error) {
	out, err := url.QueryUnescape(s)
	return out, invalid(ErrParamValue, err)
}

// keepValue reads a cookie value, which is not percent-encoded.
func keepValue(s string) (string, error) {
	return s, nil
}

func readAll(values []string, read func(string) (string, error)) ([]string, error) {
	out := make([]string, len(values))
	for i, v := range values {
		s, err := read(v)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
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
func nested(q Query, name string) (map[string]any, error) {
	var out map[string]any
	for _, key := range slices.Sorted(maps.Keys(q)) {
		rest, ok := strings.CutPrefix(key, name+"[")
		if !ok || len(q[key]) == 0 {
			continue
		}
		values, err := readAll(q[key], unescapeQuery)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = map[string]any{}
		}
		setPath(out, splitBrackets("["+rest), values)
	}
	return out, nil
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

// setPath stores values at path in m: a list under the last name, or its first value alone. An
// empty path, of a key that names nothing, stores nothing.
func setPath(m map[string]any, path []string, values []string) {
	if len(path) == 0 {
		return
	}
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

func firstValues(q map[string][]string, read func(string) (string, error)) (map[string]string, error) {
	out := make(map[string]string, len(q))
	for _, key := range slices.Sorted(maps.Keys(q)) {
		if len(q[key]) == 0 {
			continue
		}
		value, err := read(q[key][0])
		if err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, nil
}

// missing is a parameter that is not there: an error when it is required, else its default.
func missing(p Param, target reflect.Value) error {
	if p.IsRequired || p.Default == "" {
		return absent(p)
	}
	return setParamJSON(target, p.Default)
}

func absent(p Param) error {
	if p.IsRequired {
		return fmt.Errorf("%w: %s", ErrParamMissing, p.Name)
	}
	return nil
}

// setParam stores v in target as the assigner does, its error marked a bad parameter value.
func setParam(target reflect.Value, v any) error {
	return invalid(ErrParamValue, assigner{}.assign(target, v))
}

// setParamJSON decodes raw as JSON into target, its error marked a bad parameter value.
func setParamJSON(target reflect.Value, raw string) error {
	return invalid(ErrParamValue, assigner{}.json(target, raw))
}

// pointer returns what dst points to, ready to be set.
func pointer(dst any) (reflect.Value, error) {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return reflect.Value{}, fmt.Errorf("%w: the target must be a pointer", ErrParamValue)
	}
	return v.Elem(), nil
}

// allocate follows v through pointers, making each nil one point at a new value, and returns what
// it reaches.
func allocate(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	return v
}

// shapeOf is what a Go type holds as a parameter. A type that reads text on its own, such as
// time.Time, is one value.
func shapeOf(t reflect.Type) shape {
	t = valueType(t)
	switch {
	case reflect.PointerTo(t).Implements(textUnmarshaler):
		return shapeValue
	case t.Kind() == reflect.Slice && !isBytes(t):
		return shapeList
	case t.Kind() == reflect.Struct || t.Kind() == reflect.Map:
		return shapeObject
	}
	return shapeValue
}

// isBytes reports a slice of bytes, which text carries as base64.
func isBytes(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8
}

// encodeTree writes v as text: one string, a list of strings, or name-value pairs, which a
// deepObject names by their whole key. A nil pointer or a Nullable without a value gives nil.
func encodeTree(v any, p Param) (any, error) {
	rv, ok := held(reflect.ValueOf(v))
	if !ok {
		return nil, nil
	}
	if p.IsJSON {
		data, err := json.Marshal(rv.Interface())
		return string(data), err
	}

	switch shapeOf(rv.Type()) {
	case shapeList:
		return itemTexts(rv)
	case shapeObject:
		if p.Style == StyleDeepObject {
			return deepPairs(p.Name, rv)
		}
		return pairs(rv)
	default:
		return text(rv)
	}
}

func itemTexts(rv reflect.Value) ([]string, error) {
	out := make([]string, rv.Len())
	for i := range rv.Len() {
		s, err := text(rv.Index(i))
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

func pairs(rv reflect.Value) ([]pair, error) {
	var out []pair
	for name, v := range properties(rv) {
		s, err := text(v)
		if err != nil {
			return nil, err
		}
		out = append(out, pair{name: name, value: s})
	}
	return out, nil
}

// deepPairs writes the object rv under key as deepObject nests it: key[name] for each property,
// at any depth, and one pair per item of a list.
func deepPairs(key string, rv reflect.Value) ([]pair, error) {
	var out []pair
	for name, v := range properties(rv) {
		field := key + "[" + name + "]"
		switch shapeOf(v.Type()) {
		case shapeObject:
			inner, err := deepPairs(field, v)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
		case shapeList:
			texts, err := itemTexts(v)
			if err != nil {
				return nil, err
			}
			for _, s := range texts {
				out = append(out, pair{name: field, value: s})
			}
		default:
			s, err := text(v)
			if err != nil {
				return nil, err
			}
			out = append(out, pair{name: field, value: s})
		}
	}
	return out, nil
}

// properties yields the name and value of each property of rv, a struct or a map, in the order
// they are written. An unset property is left out: a nil pointer or interface, or a list or map
// with no items.
func properties(rv reflect.Value) iter.Seq2[string, reflect.Value] {
	return func(yield func(string, reflect.Value) bool) {
		if rv.Kind() == reflect.Map {
			for _, key := range sortedMapKeys(rv) {
				if v, ok := present(rv.MapIndex(key)); ok && !yield(key.String(), v) {
					return
				}
			}
			return
		}
		for i := range rv.NumField() {
			f := rv.Type().Field(i)
			name := jsonName(f)
			if name == "" || !f.IsExported() {
				continue
			}
			if v, ok := present(rv.Field(i)); ok && !yield(name, v) {
				return
			}
		}
	}
}

// present is what v holds behind pointers, interfaces and Nullables, and false when that is
// nothing or a list or map with no items.
func present(v reflect.Value) (reflect.Value, bool) {
	v, ok := held(v)
	isEmpty := (v.Kind() == reflect.Slice || v.Kind() == reflect.Map) && v.Len() == 0
	return v, ok && !isEmpty
}

// text writes one value as a parameter carries it.
func text(v reflect.Value) (string, error) {
	if v.Type().Implements(textMarshaler) {
		m, _ := v.Interface().(encoding.TextMarshaler)
		b, err := m.MarshalText()
		return string(b), err
	}
	if isBytes(v.Type()) {
		return base64.StdEncoding.EncodeToString(v.Bytes()), nil
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

// escapeTree percent-encodes every name and value of t for a query.
func escapeTree(t any, isReserved bool) any {
	switch t := t.(type) {
	case string:
		return escapeQuery(t, isReserved)
	case []string:
		out := make([]string, len(t))
		for i, s := range t {
			out[i] = escapeQuery(s, isReserved)
		}
		return out
	case []pair:
		out := make([]pair, len(t))
		for i, f := range t {
			out[i] = pair{name: escapeQuery(f.name, isReserved), value: escapeQuery(f.value, isReserved)}
		}
		return out
	}
	return nil
}

// escapeQuery percent-encodes every byte of s but letters, digits and -._~. isReserved keeps the
// reserved characters a query holds too, and a %XX escape.
func escapeQuery(s string, isReserved bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := range len(s) {
		c := s[i]
		if isUnreserved(c) || isReserved && (strings.IndexByte(queryReserved, c) >= 0 || c == '%' && isEscape(s[i:])) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperHex[c>>4])
		b.WriteByte(upperHex[c&15])
	}
	return b.String()
}

// isUnreserved reports a byte RFC 3986 never escapes: a letter, a digit, or one of -._~.
func isUnreserved(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("-._~", c) >= 0
}

// isEscape reports whether s starts with a %XX escape.
func isEscape(s string) bool {
	const hex = "0123456789ABCDEFabcdef"
	return len(s) >= 3 && strings.IndexByte(hex, s[1]) >= 0 && strings.IndexByte(hex, s[2]) >= 0
}
