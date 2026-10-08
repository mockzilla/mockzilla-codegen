// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stamped struct {
	Title    string         `json:"title"`
	When     time.Time      `json:"when"`
	Raw      []byte         `json:"raw"`
	Optional *string        `json:"optional"`
	Any      any            `json:"any"`
	Ptrs     []*string      `json:"ptrs"`
	Meta     map[string]int `json:"meta"`
	Note     string         `json:"note,omitempty"`
	Since    time.Time      `json:"since,omitempty,omitzero"`
	Bad      chan int       `json:"-"`
}

// part is one part of a multipart form as it went out.
type part struct {
	Name        string
	ContentType string
	Body        string
}

// failAfter takes n bytes and fails the write that goes past them.
type failAfter struct {
	n int
}

func (w *failAfter) Write(p []byte) (int, error) {
	if len(p) > w.n {
		return 0, io.ErrClosedPipe
	}
	w.n -= len(p)
	return len(p), nil
}

// textForm stands in for a union that reads forms and holds a string.
type textForm struct {
	Text string
}

func (f textForm) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.Text)
}

func (*textForm) UnmarshalForm(*multipart.Form) error {
	return nil
}

func TestEncodeForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    url.Values
		wantErr string
	}{
		{
			name: "Nested objects, lists and lists of objects, in the shape DecodeForm reads",
			value: order{
				Name:    "Rex",
				Age:     Ptr(3),
				Active:  true,
				Address: address{City: "Berlin", Country: "DE"},
				Items:   []string{"a", "b"},
				Lines:   []address{{City: "Paris"}},
				Extra:   map[string]any{"n": 1.5, "list": []any{[]any{1, 2}}},
				Ignored: "gone",
			},
			want: url.Values{
				"name": {"Rex"}, "age": {"3"}, "active": {"true"},
				"address[city]": {"Berlin"}, "address[country]": {"DE"},
				"items":          {"a", "b"},
				"lines[0][city]": {"Paris"}, "lines[0][country]": {""},
				"extra[n]": {"1.5"}, "extra[list][0]": {"1", "2"},
				"phone": {""},
			},
		},
		{name: "A map", value: map[string]int{"b": 2, "a": 1}, want: url.Values{"a": {"1"}, "b": {"2"}}},
		{name: "Nothing", value: nil, want: url.Values{}},
		{name: "A value that is no object", value: []int{1}, wantErr: "invalid body value: a form needs an object, not \"[1]\""},
		{name: "A value that cannot be written as JSON", value: make(chan int), wantErr: "json: unsupported type: chan int"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := EncodeForm(tc.value, nil)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEncodeFormJSONValues(t *testing.T) {
	t.Parallel()

	in := drawing{
		Vertex:   &vertex{Point: &point{X: 7}},
		Vertices: []vertex{{Text: new("a")}, {Point: &point{X: 1}}},
		Named:    map[string]vertex{"n": {Point: &point{X: 2}}},
		Origin:   &point{X: 3},
	}
	values, err := EncodeForm(in, nil)
	require.NoError(t, err)
	assert.Equal(t, url.Values{
		"vertex":    {`{"x":7}`},
		"vertices":  {"a", `{"x":1}`},
		"named[n]":  {`{"x":2}`},
		"origin[x]": {"3"},
	}, values)

	var out drawing
	require.NoError(t, DecodeForm(strings.NewReader(values.Encode()), &out, false, nil))
	assert.Equal(t, in, out)

	raw, err := EncodeForm(json.RawMessage(`{"a":{"b":1}}`), nil)
	require.NoError(t, err)
	assert.Equal(t, url.Values{"a[b]": {"1"}}, raw)
}

func TestEncodeFormRoundTrip(t *testing.T) {
	t.Parallel()

	in := order{
		Name:    "Rex",
		Age:     Ptr(3),
		Active:  true,
		Address: address{City: "Berlin", Country: "DE"},
		Items:   []string{"a", "b"},
		Lines:   []address{{City: "Paris"}, {City: "Rome"}},
		Extra:   map[string]any{"n": int64(1)},
		Codes:   []int{7},
		Meta:    map[string]bool{"x": true},
	}
	values, err := EncodeForm(in, nil)
	require.NoError(t, err)

	var out order
	require.NoError(t, DecodeForm(strings.NewReader(values.Encode()), &out, true, nil))

	assert.Equal(t, in, out)
}

func TestEncodeFormEncoding(t *testing.T) {
	t.Parallel()

	in := parcel{ID: Ptr("p1"), Pet: &address{City: "Rome"}, Pets: []address{{City: "A"}}, Tags: []string{"x", "y"}, Note: "hi"}
	got, err := EncodeForm(in, Encoding{"id": "application/json", "pet": "application/json", "pets": "application/json", "note": "text/plain"})
	require.NoError(t, err)
	assert.Equal(t, `id=%22p1%22&note=hi&pet=%7B%22city%22%3A%22Rome%22%2C%22country%22%3A%22%22%7D&pets=%5B%7B%22city%22%3A%22A%22%2C%22country%22%3A%22%22%7D%5D&tags=x&tags=y`, got.Encode())

	_, err = EncodeForm(in, Encoding{"pet": "text/plain"})
	require.EqualError(t, err, "unsupported content type: pet in text/plain")
	_, err = EncodeForm(in, Encoding{"tags": "text/plain"})
	require.NoError(t, err)
}

func TestWriteMultipart(t *testing.T) {
	t.Parallel()

	in := upload{
		Title:    "Cat",
		File:     NewFile([]byte("meow"), "cat.txt", "text/plain"),
		Optional: nil,
		Files:    []File{NewFile([]byte("a"), "a.bin", ""), NewFile([]byte("b"), "", "")},
		Tags:     []string{"x", "y"},
		Address:  &address{City: "Berlin"},
		Point:    address{City: "Rome"},
		Count:    3,
	}

	data, contentType := multipartOf(t, &in)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
	req.Header.Set("Content-Type", contentType)
	var out upload
	require.NoError(t, DecodeMultipart(req, &out, 0, nil))
	content, err := out.File.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "meow", string(content))
	assert.Equal(t, "cat.txt", out.File.Name())
	assert.Equal(t, "text/plain", out.File.ContentType())
	require.Len(t, out.Files, 2)
	assert.Equal(t, "application/octet-stream", out.Files[0].ContentType())
	assert.Equal(t, "blob", out.Files[1].Name())
	assert.Nil(t, out.Optional)
	assert.Equal(t, in.Tags, out.Tags)
	assert.Equal(t, in.Address, out.Address)
	assert.Equal(t, in.Point, out.Point)
	assert.Equal(t, 3, out.Count)
	assert.Equal(t, "Cat", out.Title)
}

func TestWriteMultipartParts(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name  string
		value stamped
		want  map[string][]string
	}{
		{
			name:  "Each set field is a part of its own",
			value: stamped{Title: `a "quoted" \ name`, When: when, Raw: []byte{0, 1}, Any: map[string]int{"n": 1}, Ptrs: []*string{Ptr("p"), nil}, Meta: map[string]int{}},
			want: map[string][]string{
				"title": {`a "quoted" \ name`},
				"when":  {"2026-01-02T03:04:05Z"},
				"raw":   {"AAE="},
				"any":   {`{"n":1}`},
				"ptrs":  {"p"},
				"meta":  {"{}"},
			},
		},
		{
			name:  "A nil list, bytes or map is left out",
			value: stamped{Title: "x", When: when},
			want:  map[string][]string{"title": {"x"}, "when": {"2026-01-02T03:04:05Z"}},
		},
		{
			name:  "Empty bytes are an empty part",
			value: stamped{Title: "x", When: when, Raw: []byte{}},
			want:  map[string][]string{"title": {"x"}, "when": {"2026-01-02T03:04:05Z"}, "raw": {""}},
		},
		{
			name:  "A zero field tagged omitempty or omitzero is left out, an untagged one written",
			value: stamped{Note: "n"},
			want:  map[string][]string{"title": {""}, "when": {"0001-01-01T00:00:00Z"}, "note": {"n"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, contentType := multipartOf(t, tc.value)
			_, params, err := mime.ParseMediaType(contentType)
			require.NoError(t, err)
			mr := multipart.NewReader(bytes.NewReader(data), params["boundary"])
			form, err := mr.ReadForm(1 << 20)
			require.NoError(t, err)

			assert.Equal(t, tc.want, form.Value)
		})
	}
}

func TestWriteMultipartUnmarshaler(t *testing.T) {
	t.Parallel()

	p := post{ID: "1", Photo: &photo{Image: NewFileReader(strings.NewReader("PNG"), "a.png", "image/png", 3), Caption: "sun", Tags: []string{"a", "b"}}}
	size, boundary, err := MultipartSize(p, nil)
	require.NoError(t, err)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.SetBoundary(boundary))
	require.NoError(t, WriteMultipart(mw, &p, nil))
	assert.Equal(t, int64(buf.Len()), size)

	form, err := multipart.NewReader(&buf, boundary).ReadForm(1 << 20)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"id": {"1"}, "caption": {"sun"}, "tags": {"a", "b"}}, form.Value)
	require.Len(t, form.File["image"], 1)
	assert.Equal(t, "a.png", form.File["image"][0].Filename)
	data, err := NewFileFromMultipart(form.File["image"][0]).Bytes()
	require.NoError(t, err)
	assert.Equal(t, "PNG", string(data))
}

func TestWriteMultipartMembers(t *testing.T) {
	t.Parallel()

	g := tagged{Name: "n", Extra: map[string]any{
		"none": nil, "num": 1.5, "yes": true, "text": "t", "obj": map[string]int{"k": 1},
		"list": []any{1, "a"}, "objs": []any{map[string]int{"k": 1}}, "lists": []any{[]int{1}},
	}}
	data, contentType := multipartOf(t, g)

	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	form, err := multipart.NewReader(bytes.NewReader(data), params["boundary"]).ReadForm(1 << 20)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{
		"name": {"n"}, "num": {"1.5"}, "yes": {"true"}, "text": {"t"}, "obj": {`{"k":1}`},
		"list": {"1", "a"}, "objs": {`{"k":1}`}, "lists": {`[1]`},
	}, form.Value)

	list, contentType := multipartOf(t, struct {
		List [][]byte `json:"list"`
	}{List: [][]byte{[]byte("a"), []byte("b")}})
	_, params, err = mime.ParseMediaType(contentType)
	require.NoError(t, err)
	form, err = multipart.NewReader(bytes.NewReader(list), params["boundary"]).ReadForm(1 << 20)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"list": {"YQ==", "Yg=="}}, form.Value)
}

func TestWriteMultipartEncoding(t *testing.T) {
	t.Parallel()

	in := parcel{
		ID:    Ptr("p1"),
		Doc:   new(NewFile([]byte("%PDF"), "a.pdf", "")),
		Logo:  new(NewFile([]byte("PNG"), "logo.png", "image/png")),
		Pet:   &address{City: "Rome"},
		Pets:  []address{{City: "A"}, {City: "B"}},
		Tags:  []string{"x"},
		Note:  "hi",
		Plain: []address{{City: "C"}},
	}
	got := partsOf(t, in, parcelEncoding())

	assert.Equal(t, []part{
		{Name: "id", ContentType: "application/json", Body: `"p1"`},
		{Name: "doc", ContentType: "application/pdf", Body: "%PDF"},
		{Name: "logo", ContentType: "image/png", Body: "PNG"},
		{Name: "pet", ContentType: "application/json", Body: `{"city":"Rome","country":""}`},
		{Name: "pets", ContentType: "application/json", Body: `{"city":"A","country":""}`},
		{Name: "pets", ContentType: "application/json", Body: `{"city":"B","country":""}`},
		{Name: "tags", ContentType: "application/json", Body: `"x"`},
		{Name: "note", ContentType: "text/plain; charset=utf-8", Body: "hi"},
		{Name: "plain", ContentType: "application/json", Body: `{"city":"C","country":""}`},
	}, got)

	in.Logo = new(NewFile([]byte("PNG"), "logo", ""))
	err := WriteMultipart(multipart.NewWriter(io.Discard), in, parcelEncoding())
	require.EqualError(t, err, "invalid body value: logo: the file has no content type; the spec takes image/png, image/jpeg")

	err = WriteMultipart(multipart.NewWriter(io.Discard), parcel{Pet: &address{}}, Encoding{"pet": "application/xml"})
	require.EqualError(t, err, "unsupported content type: pet in application/xml")
	err = WriteMultipart(multipart.NewWriter(io.Discard), parcel{Note: "x"}, Encoding{"note": "image/*"})
	require.EqualError(t, err, "unsupported content type: note in image/*")
}

func TestWriteMultipartErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		wantErr string
	}{
		{name: "No struct", value: 42, wantErr: "invalid body value: a multipart form needs a struct, not int"},
		{name: "A nil pointer", value: (*upload)(nil), wantErr: "invalid body value: a multipart form needs a struct, not *runtime.upload"},
		{name: "A file that cannot be opened", value: upload{File: NewFileFromMultipart(&multipart.FileHeader{Filename: "gone"})}, wantErr: "open : no such file or directory"},
		{name: "A value that cannot be written as JSON", value: struct{ Any any }{Any: map[string]chan int{"a": nil}}, wantErr: "json: unsupported type: chan int"},
		{name: "A value that cannot be written as text", value: struct{ Bad chan int }{}, wantErr: "invalid parameter value: cannot write chan int as a parameter"},
		{name: "A file that cannot be read", value: upload{File: NewFileReader(errReader{}, "a", "", -1)}, wantErr: "unexpected EOF"},
		{name: "A file of a list that cannot be read", value: upload{Files: []File{NewFileReader(errReader{}, "a", "", -1)}}, wantErr: "unexpected EOF"},
		{name: "A form type that cannot be written as JSON", value: tagged{Extra: map[string]any{"a": make(chan int)}}, wantErr: `json: error calling MarshalJSON for type runtime.tagged: invalid additional property "a": json: unsupported type: chan int`},
		{name: "A form type that is no object", value: textForm{Text: "hello"}, wantErr: `invalid body value: a multipart form needs an object, not "\"hello\""`},
		{name: "A file of a union that cannot be read", value: post{Photo: &photo{Image: NewFileReader(errReader{}, "a", "", -1)}}, wantErr: "unexpected EOF"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := WriteMultipart(multipart.NewWriter(io.Discard), tc.value, nil)

			require.EqualError(t, err, tc.wantErr)
		})
	}
}

// TestWriteMultipartFailingWriter fails the writer at every point a part writes to it.
func TestWriteMultipartFailingWriter(t *testing.T) {
	t.Parallel()

	values := []any{
		upload{Title: "Cat", File: NewFile([]byte("meow"), "cat.txt", ""), Point: address{City: "Rome"}},
		stamped{Raw: []byte{1}},
		tagged{Name: "n", Extra: map[string]any{"list": []int{1, 2}, "obj": map[string]int{"k": 1}}},
	}
	for _, v := range values {
		data, _ := multipartOf(t, v)

		for n := range len(data) {
			err := WriteMultipart(multipart.NewWriter(&failAfter{n: n}), v, nil)
			require.ErrorIs(t, err, io.ErrClosedPipe, n)
		}
	}
}

func TestMultipartSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     any
		wantSized bool
		wantErr   error
	}{
		{name: "Files that know their size", value: upload{Title: "Cat", File: NewFile([]byte("meow"), "cat.txt", ""), Files: []File{NewFile([]byte("abc"), "a", "")}}, wantSized: true},
		{name: "No files", value: stamped{Title: "x"}, wantSized: true},
		{name: "A file of unknown size", value: upload{File: NewFile([]byte("a"), "a", ""), Files: []File{NewFileReader(strings.NewReader("b"), "b", "", -1)}}},
		{name: "No struct", value: 42, wantErr: ErrBodyValue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			size, boundary, err := MultipartSize(tc.value, nil)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if !tc.wantSized {
				assert.Equal(t, int64(-1), size)
				return
			}
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			require.NoError(t, mw.SetBoundary(boundary))
			require.NoError(t, WriteMultipart(mw, tc.value, nil))
			assert.Equal(t, int64(buf.Len()), size)
		})
	}
}

func TestEncodeText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		want    string
		wantErr error
	}{
		{name: "A string", value: "hi", want: "hi"},
		{name: "A number behind a pointer", value: Ptr(42), want: "42"},
		{name: "A Nullable", value: Some(true), want: "true"},
		{name: "A time by its MarshalText", value: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), want: "2026-01-02T03:04:05Z"},
		{name: "Nil", value: (*int)(nil)},
		{name: "Null", value: Null[int]()},
		{name: "A struct", value: address{}, wantErr: ErrContentType},
		{name: "A value text cannot carry", value: make(chan int), wantErr: ErrParamValue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := EncodeText(tc.value)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEncodingRoundTrip(t *testing.T) {
	t.Parallel()

	in := parcel{
		ID:    Ptr("p1"),
		Doc:   new(NewFile([]byte("%PDF"), "a.pdf", "")),
		Logo:  new(NewFile([]byte("PNG"), "logo.png", "image/png")),
		Pet:   &address{City: "Rome"},
		Pets:  []address{{City: "A"}, {City: "B"}},
		Tags:  []string{"x", "y"},
		Note:  "hi",
		Plain: []address{{City: "C"}, {City: "D"}},
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, WriteMultipart(mw, in, parcelEncoding()))
	r := httptest.NewRequest(http.MethodPost, "/", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	var out parcel
	require.NoError(t, DecodeMultipart(r, &out, 0, parcelEncoding()))
	assert.Equal(t, "p1", *out.ID)
	assert.Equal(t, "application/pdf", out.Doc.ContentType())
	assert.Equal(t, in.Pet, out.Pet)
	assert.Equal(t, in.Pets, out.Pets)
	assert.Equal(t, in.Tags, out.Tags)
	assert.Equal(t, "hi", out.Note)
	assert.Equal(t, in.Plain, out.Plain)

	in.Doc, in.Logo = nil, nil
	enc := Encoding{"id": "application/json", "pet": "application/json", "pets": "application/json", "tags": "application/json"}
	values, err := EncodeForm(in, enc)
	require.NoError(t, err)
	var form parcel
	require.NoError(t, DecodeForm(strings.NewReader(values.Encode()), &form, true, enc))
	assert.Equal(t, in, form)
}

// multipartOf writes v as a multipart form and returns it with its content type.
func multipartOf(t *testing.T, v any) ([]byte, string) {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, WriteMultipart(mw, v, nil))
	return buf.Bytes(), mw.FormDataContentType()
}

func parcelEncoding() Encoding {
	return Encoding{
		"id": "application/json", "doc": "application/pdf", "logo": "image/png, image/jpeg", "pet": "application/json",
		"pets": "application/json", "tags": "application/json", "note": "text/plain; charset=utf-8",
	}
}

// partsOf writes v as a multipart form with enc and returns its parts in order.
func partsOf(t *testing.T, v any, enc Encoding) []part {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, WriteMultipart(mw, v, enc))
	_, params, err := mime.ParseMediaType(mw.FormDataContentType())
	require.NoError(t, err)

	var out []part
	mr := multipart.NewReader(&buf, params["boundary"])
	for {
		p, nextErr := mr.NextPart()
		if errors.Is(nextErr, io.EOF) {
			return out
		}
		require.NoError(t, nextErr)
		data, readErr := io.ReadAll(p)
		require.NoError(t, readErr)
		out = append(out, part{Name: p.FormName(), ContentType: p.Header.Get("Content-Type"), Body: string(data)})
	}
}
