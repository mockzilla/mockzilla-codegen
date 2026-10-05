// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// What the server checks and fills in a request body before it decodes it.

package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Bodies are the objects of request bodies by name; IsChecked adds the checks to the defaults.
type Bodies struct {
	IsChecked bool
	Objects   map[string]Object
}

// Object is an object schema: its properties, what other keys hold, and whether they are errors.
type Object struct {
	Props    map[string]Prop
	Extra    *Prop
	IsClosed bool
}

// Prop is one value of a body: a property, a list item, a map value or the whole body.
type Prop struct {
	IsRequired bool
	IsNullable bool
	Default    string
	Object     string
	Items      *Prop
	Values     *Prop
}

type walker struct {
	bodies    Bodies
	errs      ValidationErrors
	isChanged bool
}

// formAt is where a form value sits: its path in errors, and its key in the form values.
type formAt struct {
	path   string
	key    string
	values url.Values
}

// JSON checks a JSON body against p, sets the defaults it lacks and returns the body to decode.
func (b Bodies) JSON(body io.Reader, p Prop) (io.Reader, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	v, isJSON := parseJSON(data)
	if !isJSON {
		return bytes.NewReader(data), nil // the decoder reports what is empty or no JSON
	}

	w := &walker{bodies: b}
	w.value(v, p, "body")
	if len(w.errs) > 0 {
		return nil, w.errs
	}
	if w.isChanged {
		data = encodeJSON(v)
	}
	return bytes.NewReader(data), nil
}

// Form checks a url-encoded form against the object p names and returns the form to decode.
func (b Bodies) Form(body io.Reader, p Prop) (io.Reader, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	values, err := url.ParseQuery(string(data))
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return bytes.NewReader(data), nil
	}

	w := &walker{bodies: b}
	w.form(values, nil, p)
	if len(w.errs) > 0 {
		return nil, w.errs
	}
	if w.isChanged {
		data = []byte(values.Encode())
	}
	return bytes.NewReader(data), nil
}

// Multipart parses the multipart form of r, checks it against the object p names and fills it.
func (b Bodies) Multipart(r *http.Request, p Prop, maxMemory int64) error {
	if maxMemory <= 0 {
		maxMemory = DefaultMultipartMemory
	}
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		return err
	}

	w := &walker{bodies: b}
	w.form(r.MultipartForm.Value, r.MultipartForm.File, p)
	return w.errs.Err()
}

// IsValidation reports whether err holds ValidationErrors.
func IsValidation(err error) bool {
	var errs ValidationErrors
	return errors.As(err, &errs)
}

func (w *walker) value(v any, p Prop, path string) {
	switch x := v.(type) {
	case nil:
		if w.bodies.IsChecked && !p.IsNullable {
			w.errs.Add(path, "must not be null")
		}
	case map[string]any:
		if o, isObject := w.bodies.Objects[p.Object]; isObject {
			w.object(x, o, path)
			return
		}
		if p.Values != nil {
			for _, key := range SortedKeys(x) {
				w.value(x[key], *p.Values, Key(path, key))
			}
		}
	case []any:
		if p.Items != nil {
			for i, item := range x {
				w.value(item, *p.Items, Index(path, i))
			}
		}
	}
}

func (w *walker) object(m map[string]any, o Object, path string) {
	for _, name := range SortedKeys(o.Props) {
		p := o.Props[name]
		v, isSet := m[name]
		switch {
		case isSet:
			w.value(v, p, joinPath(path, name))
		case p.IsRequired && w.bodies.IsChecked:
			w.errs.Add(joinPath(path, name), "is required")
		case p.Default != "":
			if d, isJSON := parseJSON([]byte(p.Default)); isJSON {
				m[name], w.isChanged = d, true
			}
		}
	}

	for _, key := range SortedKeys(m) {
		if _, isProp := o.Props[key]; isProp {
			continue
		}
		switch {
		case o.IsClosed && w.bodies.IsChecked:
			w.errs.Add(joinPath(path, key), "is not allowed")
		case o.Extra != nil:
			w.value(m[key], *o.Extra, Key(path, key))
		}
	}
}

// form checks a form against the object p names; a file part counts as its field.
func (w *walker) form(values url.Values, files map[string][]*multipart.FileHeader, p Prop) {
	o, isObject := w.bodies.Objects[p.Object]
	if !isObject {
		return
	}

	fields := formTree(values)
	for name := range files {
		if _, isSet := fields[name]; !isSet {
			fields[name] = []string(nil)
		}
	}
	w.formObject(fields, o, formAt{path: "body", values: values})
}

func (w *walker) formObject(m map[string]any, o Object, at formAt) {
	for _, name := range SortedKeys(o.Props) {
		p := o.Props[name]
		child, isSet := m[name]
		switch {
		case isSet:
			w.formValue(child, p, at.field(name))
		case p.IsRequired && w.bodies.IsChecked:
			w.errs.Add(joinPath(at.path, name), "is required")
		case p.Default != "":
			w.formDefault(at.field(name), p.Default)
		}
	}

	for _, key := range SortedKeys(m) {
		if _, isProp := o.Props[key]; isProp {
			continue
		}
		switch {
		case o.IsClosed && w.bodies.IsChecked:
			w.errs.Add(joinPath(at.path, key), "is not allowed")
		case o.Extra != nil:
			w.formValue(m[key], *o.Extra, at.entry(key))
		}
	}
}

func (w *walker) formValue(node any, p Prop, at formAt) {
	switch n := node.(type) {
	case string:
		w.formJSON(n, p, at)
	case map[string]any:
		if o, isObject := w.bodies.Objects[p.Object]; isObject {
			w.formObject(n, o, at)
			return
		}
		if p.Values != nil {
			for _, key := range SortedKeys(n) {
				w.formValue(n[key], *p.Values, at.entry(key))
			}
		}
	case []any:
		if p.Items != nil {
			for i, item := range n {
				w.formValue(item, *p.Items, at.item(i))
			}
		}
	}
}

// formJSON checks a field that holds a JSON object or array as JSON, as DecodeForm reads it.
func (w *walker) formJSON(text string, p Prop, at formAt) {
	trimmed := strings.TrimSpace(text)
	if p.Object == "" && p.Items == nil && p.Values == nil || !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return
	}
	v, isJSON := parseJSON([]byte(trimmed))
	if !isJSON {
		return
	}

	inner := &walker{bodies: w.bodies}
	inner.value(v, p, at.path)
	w.errs = append(w.errs, inner.errs...)
	if inner.isChanged {
		at.values[at.key] = []string{string(encodeJSON(v))}
		w.isChanged = true
	}
}

// formDefault adds a default as form text: a list of scalars item by item, the rest as JSON.
func (w *walker) formDefault(at formAt, def string) {
	v, isJSON := parseJSON([]byte(def))
	if !isJSON || v == nil {
		return
	}

	switch x := v.(type) {
	case map[string]any:
		at.values.Add(at.key, string(encodeJSON(x)))
	case []any:
		if slices.ContainsFunc(x, isComposite) {
			at.values.Add(at.key, string(encodeJSON(x)))
			break
		}
		for _, item := range x {
			at.values.Add(at.key, formText(item))
		}
	default:
		at.values.Add(at.key, formText(x))
	}
	w.isChanged = true
}

func (a formAt) field(name string) formAt {
	return formAt{path: joinPath(a.path, name), key: a.nested(name), values: a.values}
}

func (a formAt) entry(key string) formAt {
	return formAt{path: Key(a.path, key), key: a.nested(key), values: a.values}
}

func (a formAt) item(i int) formAt {
	return formAt{path: Index(a.path, i), key: a.nested(strconv.Itoa(i)), values: a.values}
}

func (a formAt) nested(name string) string {
	if a.key == "" {
		return name
	}
	return a.key + "[" + name + "]"
}

// parseJSON reads one JSON value with its numbers as written, and reports whether data is one.
func parseJSON(data []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	_, err := dec.Token()
	return v, errors.Is(err, io.EOF)
}

func encodeJSON(v any) []byte {
	data, _ := json.Marshal(v) // what parseJSON reads always marshals
	return data
}

func isComposite(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}
