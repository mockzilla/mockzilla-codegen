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
// apply.
func EncodeForm(v any) (url.Values, error) {
	fields, err := jsonObject(v)
	if err != nil {
		return nil, err
	}

	out := url.Values{}
	addForm(out, "", fields)
	return out, nil
}

// EncodeMultipart writes v, a struct, as a multipart form: File fields as file parts with their
// name and content type, application/octet-stream without one, structs, maps and lists of them as JSON parts, lists of values as
// repeated parts, bytes as they are, and everything else as text. It returns the body and its
// content type, which carries the boundary.
func EncodeMultipart(v any) ([]byte, string, error) {
	var buf bytes.Buffer
	contentType, err := writeMultipart(&buf, v)
	if err != nil {
		return nil, "", err
	}
	return buf.Bytes(), contentType, nil
}

// writeMultipart writes v, a struct, to w as a multipart form and returns the content type.
func writeMultipart(w io.Writer, v any) (string, error) {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return "", fmt.Errorf("%w: a multipart form needs a struct, not %T", ErrBodyValue, v)
	}

	mw := multipart.NewWriter(w)
	for i := range rv.NumField() {
		f := rv.Type().Field(i)
		name := jsonName(f)
		if name == "" || !f.IsExported() {
			continue
		}
		if err := writePart(mw, name, rv.Field(i)); err != nil {
			return "", err
		}
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	return mw.FormDataContentType(), nil
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

// writePart writes one field of a multipart form, by its type.
func writePart(mw *multipart.Writer, name string, v reflect.Value) error {
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
		return writeFile(mw, name, f)
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8:
		w, err := mw.CreateFormField(name)
		if err != nil {
			return err
		}
		_, err = w.Write(v.Bytes())
		return err
	case t.Kind() == reflect.Slice && t.Elem() == fileType, t.Kind() == reflect.Slice && isScalar(t.Elem()):
		for i := range v.Len() {
			if err := writePart(mw, name, v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	case isScalar(t):
		s, err := text(v)
		if err != nil {
			return err
		}
		return mw.WriteField(name, s)
	}

	data, err := json.Marshal(v.Interface())
	if err != nil {
		return err
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+quoteEscaper.Replace(name)+`"`)
	h.Set("Content-Type", "application/json")
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// writeFile writes f as a file part, with its name and its content type when it has one.
func writeFile(mw *multipart.Writer, name string, f File) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+quoteEscaper.Replace(name)+`"; filename="`+quoteEscaper.Replace(f.Name())+`"`)
	h.Set("Content-Type", cmp.Or(f.ContentType(), "application/octet-stream"))
	w, err := mw.CreatePart(h)
	if err != nil {
		return err
	}

	rc, err := f.Reader()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(w, rc)
	return err
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
