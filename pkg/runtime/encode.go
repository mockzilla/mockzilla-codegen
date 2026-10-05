// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// EncodeForm writes v, a struct or a map, as form values: nested objects with bracketed keys,
// address[city]=Berlin, lists as repeated keys, tags=a&tags=b, and lists of objects with an
// index, lines[0][city]=Berlin. Values go through their JSON form, so json tags and marshalers
// apply. A value that writes its own JSON object or array, such as a union, is one JSON value.
func EncodeForm(v any) (url.Values, error) {
	fields, err := jsonObject(v)
	if err != nil {
		return nil, err
	}

	out := url.Values{}
	addForm(out, "", jsonFields(reflect.ValueOf(v), fields))
	return out, nil
}

// WriteMultipart writes v, a struct, to mw as a multipart form and closes mw, which ends the
// form: File fields as file parts with their name and content type, application/octet-stream
// without one, structs, maps and lists of them as JSON parts, lists of values as repeated parts,
// bytes as base64, and everything else as text.
func WriteMultipart(mw *multipart.Writer, v any) error {
	return (&formWriter{mw: mw}).write(v)
}

// formWriter writes the parts of a multipart form. Counting, it reads no file and adds the file
// sizes to size instead; isUnsized records a file that does not know its size.
type formWriter struct {
	mw         *multipart.Writer
	isCounting bool
	size       int64
	isUnsized  bool
}

func (w *formWriter) write(v any) error {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("%w: a multipart form needs a struct, not %T", ErrBodyValue, v)
	}
	if reflect.PointerTo(rv.Type()).Implements(formUnmarshaler) {
		return w.object(rv)
	}

	for i := range rv.NumField() {
		f := rv.Type().Field(i)
		name := jsonName(f)
		if name == "" || !f.IsExported() {
			continue
		}
		if err := w.part(name, rv.Field(i)); err != nil {
			return err
		}
	}
	return w.mw.Close()
}

// part writes one field of a multipart form, by its type.
func (w *formWriter) part(name string, v reflect.Value) error {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	t := v.Type()
	switch {
	case t == fileType:
		f, _ := v.Interface().(File)
		return w.file(name, f)
	case isBytes(t), isScalar(t):
		s, err := text(v)
		if err != nil {
			return err
		}
		return w.mw.WriteField(name, s)
	case t.Kind() == reflect.Slice && (t.Elem() == fileType || isBytes(t.Elem()) || isScalar(t.Elem())):
		for i := range v.Len() {
			if err := w.part(name, v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	}

	data, err := json.Marshal(v.Interface())
	if err != nil {
		return err
	}
	return w.jsonPart(name, data)
}

// object writes a struct with UnmarshalForm member by member from its JSON, files as file parts.
func (w *formWriter) object(rv reflect.Value) error {
	files := map[string]reflect.Value{}
	data, err := json.Marshal(withoutFiles(rv, files).Interface())
	if err != nil {
		return err
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(data, &members) != nil {
		return fmt.Errorf("%w: a multipart form needs an object, not %.20q", ErrBodyValue, data)
	}

	for _, name := range slices.Sorted(maps.Keys(members)) {
		if f, isFile := files[name]; isFile {
			err = w.part(name, f)
		} else {
			err = w.member(name, members[name])
		}
		if err != nil {
			return err
		}
	}
	return w.mw.Close()
}

// member writes one JSON member: a scalar or a list of them as text, other values as a JSON part.
func (w *formWriter) member(name string, raw json.RawMessage) error {
	switch JSONKind(raw) {
	case KindNull:
		return nil
	case KindObject:
		return w.jsonPart(name, raw)
	case KindArray:
		var items []json.RawMessage
		_ = json.Unmarshal(raw, &items) // raw is an array of the JSON the struct wrote
		if slices.ContainsFunc(items, func(item json.RawMessage) bool { return JSONKind(item)&(KindObject|KindArray) != 0 }) {
			return w.jsonPart(name, raw)
		}
		for _, item := range items {
			if err := w.member(name, item); err != nil {
				return err
			}
		}
		return nil
	default:
	}

	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	return w.mw.WriteField(name, s)
}

// jsonPart writes data as a part of its own with the content type application/json.
func (w *formWriter) jsonPart(name string, data []byte) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+quoteEscaper.Replace(name)+`"`)
	h.Set("Content-Type", "application/json")
	pw, err := w.mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = pw.Write(data)
	return err
}

// file writes f as a file part, with its name and its content type when it has one. A file
// without a name goes as blob, as browsers send a Blob: most servers read an empty filename as text.
func (w *formWriter) file(name string, f File) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+quoteEscaper.Replace(name)+`"; filename="`+quoteEscaper.Replace(cmp.Or(f.Name(), "blob"))+`"`)
	h.Set("Content-Type", cmp.Or(f.ContentType(), "application/octet-stream"))
	pw, err := w.mw.CreatePart(h)
	if err != nil {
		return err
	}
	if w.isCounting {
		w.size += f.Size()
		w.isUnsized = w.isUnsized || f.Size() < 0
		return nil
	}

	rc, err := f.Reader()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(pw, rc)
	return err
}

// withoutFiles copies rv, moving its files and those of the set variants of a union to files.
func withoutFiles(rv reflect.Value, files map[string]reflect.Value) reflect.Value {
	out := reflect.New(rv.Type()).Elem()
	out.Set(rv)
	for i := range rv.NumField() {
		f := rv.Type().Field(i)
		v := rv.Field(i)
		name := jsonName(f)
		switch {
		case !f.IsExported():
		case name == "" && v.Kind() == reflect.Pointer && !v.IsNil() && v.Elem().Kind() == reflect.Struct:
			variant := reflect.New(v.Elem().Type())
			variant.Elem().Set(withoutFiles(v.Elem(), files))
			out.Field(i).Set(variant)
		case name != "" && isFileField(f.Type):
			files[name] = v
			out.Field(i).Set(reflect.Zero(f.Type))
		}
	}
	return out
}

// byteCounter is a writer that counts what it is given.
type byteCounter int64

func (c *byteCounter) Write(p []byte) (int, error) {
	*c += byteCounter(len(p))
	return len(p), nil
}

// multipartSize writes v as a multipart form without reading its files, and returns the boundary
// it used with the length of the form, -1 when a file does not know its size.
func multipartSize(v any) (int64, string, error) {
	var n byteCounter
	w := &formWriter{mw: multipart.NewWriter(&n), isCounting: true}
	if err := w.write(v); err != nil {
		return 0, "", err
	}
	if w.isUnsized {
		return -1, w.mw.Boundary(), nil
	}
	return int64(n) + w.size, w.mw.Boundary(), nil
}

// jsonObject is v as a JSON object, with numbers kept as text.
func jsonObject(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out map[string]any
	if err = dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: a form needs an object, not %.20q", ErrBodyValue, data)
	}
	return out, nil
}

// jsonFields puts the text of jsonField into encoded, the JSON of rv, for each value of rv.
func jsonFields(rv reflect.Value, encoded any) any {
	v, _ := present(rv)
	switch t := encoded.(type) {
	case map[string]any:
		if v.Kind() == reflect.Struct || v.Kind() == reflect.Map {
			for name, field := range properties(v) {
				if item, ok := t[name]; ok {
					t[name] = jsonField(field, item)
				}
			}
		}
	case []any:
		if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			for i := range min(v.Len(), len(t)) {
				t[i] = jsonField(v.Index(i), t[i])
			}
		}
	}
	return encoded
}

// jsonField is item, the JSON of v, as text when v writes its own JSON object or array.
func jsonField(v reflect.Value, item any) any {
	t := v.Type()
	if !t.Implements(jsonMarshaler) && !reflect.PointerTo(t).Implements(jsonMarshaler) {
		return jsonFields(v, item)
	}
	switch item.(type) {
	case map[string]any, []any:
		data, _ := json.Marshal(v.Interface()) // jsonObject has marshaled it already
		return string(data)
	}
	return item
}

// addForm adds v under key: an object with bracketed keys, a list as repeated keys, or as one
// value.
func addForm(out url.Values, key string, v any) {
	switch v := v.(type) {
	case map[string]any:
		for _, name := range slices.Sorted(maps.Keys(v)) {
			sub := name
			if key != "" {
				sub = key + "[" + name + "]"
			}
			addForm(out, sub, v[name])
		}
	case []any:
		for i, item := range v {
			switch item.(type) {
			case map[string]any, []any:
				addForm(out, key+"["+strconv.Itoa(i)+"]", item)
			default:
				addForm(out, key, item)
			}
		}
	case nil:
	default:
		out.Add(key, formText(v))
	}
}

// formText writes a JSON scalar as form text.
func formText(v any) string {
	switch v := v.(type) {
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	}
	s, _ := v.(string)
	return s
}

// isScalar reports a type written as one text: a string, a bool, a number, or one that writes
// its own text, such as time.Time.
func isScalar(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t.Implements(textMarshaler):
		return true
	case t.Kind() == reflect.Struct, t.Kind() == reflect.Map, t.Kind() == reflect.Slice, t.Kind() == reflect.Interface:
		return false
	}
	return true
}
