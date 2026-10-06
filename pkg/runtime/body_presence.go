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

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime/validation"
)

// PresenceChecker checks which keys a request body has and fills its defaults before decoding.
type PresenceChecker interface {
	JSON(body io.Reader, p Prop) (io.Reader, error)
	Form(body io.Reader, p Prop, enc Encoding) (io.Reader, error)
	Multipart(r *http.Request, p Prop, maxMemory int64, enc Encoding) error
}

// Presence holds the objects of request bodies, sorted by name; IsChecked adds checks to defaults.
type Presence struct {
	IsChecked bool
	Objects   []Object
}

// Object is an object schema: its properties sorted by key, what other keys hold, and IsClosed.
type Object struct {
	Name     string
	Props    []Prop
	Extra    *Prop
	IsClosed bool
}

// Prop is one value of a body: a property under its Key, a list item, a map value or the body.
type Prop struct {
	Key        string
	IsRequired bool
	IsNullable bool
	Default    string
	Object     string
	Items      *Prop
	Values     *Prop
}

var _ PresenceChecker = Presence{}

type walker struct {
	presence  Presence
	errs      validation.Errors
	isChanged bool
}

// formAt is where a form value sits: its path in errors, its key, and at the top the encoding.
type formAt struct {
	path     string
	key      string
	values   url.Values
	encoding Encoding
}

// JSON checks a JSON body against p, sets the defaults it lacks and returns the body to decode.
func (pr Presence) JSON(body io.Reader, p Prop) (io.Reader, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	v, isJSON := parseJSON(data)
	if !isJSON {
		return bytes.NewReader(data), nil // the decoder reports what is empty or no JSON
	}

	w := &walker{presence: pr}
	w.value(v, p, "body")
	if err = w.errs.Err(); err != nil {
		return nil, err
	}
	if w.isChanged {
		data = encodeJSON(v)
	}
	return bytes.NewReader(data), nil
}

// Form checks a url-encoded form against the object p names and returns the form to decode.
func (pr Presence) Form(body io.Reader, p Prop, enc Encoding) (io.Reader, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	values, err := url.ParseQuery(string(data))
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return bytes.NewReader(data), nil
	}

	w := &walker{presence: pr}
	w.form(values, nil, p, enc)
	if err = w.errs.Err(); err != nil {
		return nil, err
	}
	if w.isChanged {
		data = []byte(values.Encode())
	}
	return bytes.NewReader(data), nil
}

// Multipart parses the multipart form of r, checks it against the object p names and fills it.
func (pr Presence) Multipart(r *http.Request, p Prop, maxMemory int64, enc Encoding) error {
	if maxMemory <= 0 {
		maxMemory = DefaultMultipartMemory
	}
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		return err
	}

	w := &walker{presence: pr}
	w.form(r.MultipartForm.Value, r.MultipartForm.File, p, enc)
	return w.errs.Err()
}

func (w *walker) value(v any, p Prop, path string) {
	switch x := v.(type) {
	case nil:
		if w.presence.IsChecked && !p.IsNullable {
			w.errs = append(w.errs, validation.Error{Field: path, Message: "must not be null", Rule: validation.RuleType})
		}
	case map[string]any:
		if o, isObject := w.lookup(p.Object); isObject {
			w.object(x, o, path)
			return
		}
		if p.Values != nil {
			for _, key := range validation.SortedKeys(x) {
				w.value(x[key], *p.Values, validation.Key(path, key))
			}
		}
	case []any:
		if p.Items != nil {
			for i, item := range x {
				w.value(item, *p.Items, validation.Index(path, i))
			}
		}
	}
}

func (w *walker) object(m map[string]any, o Object, path string) {
	for _, p := range o.Props {
		v, isSet := m[p.Key]
		switch {
		case isSet:
			w.value(v, p, validation.Join(path, p.Key))
		case p.IsRequired && w.presence.IsChecked:
			w.errs.Required(validation.Join(path, p.Key))
		case p.Default != "":
			if d, isJSON := parseJSON([]byte(p.Default)); isJSON {
				m[p.Key], w.isChanged = d, true
			}
		}
	}

	for _, key := range validation.SortedKeys(m) {
		if isProp(o, key) {
			continue
		}
		switch {
		case o.IsClosed && w.presence.IsChecked:
			w.errs = append(w.errs, validation.Error{Field: validation.Join(path, key), Message: "is not allowed", Rule: validation.RuleAdditionalProperties, Limit: false})
		case o.Extra != nil:
			w.value(m[key], *o.Extra, validation.Key(path, key))
		}
	}
}

// form checks a form against the object p names; a file part counts as its field.
func (w *walker) form(values url.Values, files map[string][]*multipart.FileHeader, p Prop, enc Encoding) {
	o, isObject := w.lookup(p.Object)
	if !isObject {
		return
	}

	fields := formTree(values)
	for name := range files {
		if _, isSet := fields[name]; !isSet {
			fields[name] = []string(nil)
		}
	}
	w.formObject(fields, o, formAt{path: "body", values: values, encoding: enc})
}

func (w *walker) formObject(m map[string]any, o Object, at formAt) {
	for _, p := range o.Props {
		child, isSet := m[p.Key]
		isJSON := at.encoding.isJSON(p.Key)
		switch {
		case isSet:
			w.formValue(child, p, at.field(p.Key), isJSON)
		case p.IsRequired && w.presence.IsChecked:
			w.errs.Required(validation.Join(at.path, p.Key))
		case p.Default != "" && isJSON:
			at.values.Add(p.Key, p.Default)
			w.isChanged = true
		case p.Default != "":
			w.formDefault(at.field(p.Key), p.Default)
		}
	}

	for _, key := range validation.SortedKeys(m) {
		if isProp(o, key) {
			continue
		}
		switch {
		case o.IsClosed && w.presence.IsChecked:
			w.errs = append(w.errs, validation.Error{Field: validation.Join(at.path, key), Message: "is not allowed", Rule: validation.RuleAdditionalProperties, Limit: false})
		case o.Extra != nil:
			w.formValue(m[key], *o.Extra, at.entry(key), false)
		}
	}
}

// formValue checks a field against p; isDeclared says its texts are JSON, as the encoding says.
func (w *walker) formValue(node any, p Prop, at formAt, isDeclared bool) {
	switch n := node.(type) {
	case string:
		w.formTexts([]string{n}, p, at, isDeclared)
	case []string:
		w.formTexts(n, p, at, isDeclared)
	case map[string]any:
		if o, isObject := w.lookup(p.Object); isObject {
			w.formObject(n, o, at)
			return
		}
		if p.Values != nil {
			for _, key := range validation.SortedKeys(n) {
				w.formValue(n[key], *p.Values, at.entry(key), false)
			}
		}
	case []any:
		if p.Items != nil {
			for i, item := range n {
				w.formValue(item, *p.Items, at.item(i), false)
			}
		}
	}
}

// formTexts checks the JSON texts of a field as setJSON reads them, and writes back new defaults.
func (w *walker) formTexts(texts []string, p Prop, at formAt, isDeclared bool) {
	if !isDeclared && p.Object == "" && p.Items == nil && p.Values == nil {
		return
	}
	for i, item := range texts {
		trimmed := strings.TrimSpace(item)
		if !isDeclared && !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
			continue
		}
		v, isJSON := parseJSON([]byte(trimmed))
		if !isJSON {
			continue
		}
		prop, path := p, at.path
		if _, isList := v.([]any); p.Items != nil && !isList {
			prop, path = *p.Items, validation.Index(at.path, i)
		}
		// A key such as toys[] reaches here as toys, so there is nothing to write back to.
		if s, isChanged := w.walkJSON(v, prop, path); isChanged && i < len(at.values[at.key]) {
			at.values[at.key][i] = s
		}
	}
}

// walkJSON checks v against p and returns it written again when a default changed it.
func (w *walker) walkJSON(v any, p Prop, path string) (string, bool) {
	inner := &walker{presence: w.presence}
	inner.value(v, p, path)
	w.errs = append(w.errs, inner.errs...)
	if !inner.isChanged {
		return "", false
	}
	w.isChanged = true
	return string(encodeJSON(v)), true
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

// lookup finds the object of a name by binary search, as Objects is sorted by name.
func (w *walker) lookup(name string) (Object, bool) {
	i, isFound := slices.BinarySearchFunc(w.presence.Objects, name, func(o Object, target string) int { return strings.Compare(o.Name, target) })
	if !isFound {
		return Object{}, false
	}
	return w.presence.Objects[i], true
}

func (a formAt) field(name string) formAt {
	return formAt{path: validation.Join(a.path, name), key: a.nested(name), values: a.values}
}

func (a formAt) entry(key string) formAt {
	return formAt{path: validation.Key(a.path, key), key: a.nested(key), values: a.values}
}

func (a formAt) item(i int) formAt {
	return formAt{path: validation.Index(a.path, i), key: a.nested(strconv.Itoa(i)), values: a.values}
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

// isProp reports whether key is a property of o, whose Props are sorted by key.
func isProp(o Object, key string) bool {
	_, isFound := slices.BinarySearchFunc(o.Props, key, func(p Prop, target string) int { return strings.Compare(p.Key, target) })
	return isFound
}

func isComposite(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}
