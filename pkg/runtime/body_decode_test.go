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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type address struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

type order struct {
	Name    string          `json:"name"`
	Age     *int            `json:"age,omitempty"`
	Active  bool            `json:"active"`
	Address address         `json:"address"`
	Items   []string        `json:"items"`
	Lines   []address       `json:"lines"`
	Extra   map[string]any  `json:"extra"`
	Phone   string          `json:"phone"`
	Codes   []int           `json:"codes"`
	Meta    map[string]bool `json:"meta"`
	Ignored string          `json:"-"`
}

type upload struct {
	Title    string   `json:"title"`
	File     File     `json:"file"`
	Optional *File    `json:"optional"`
	Files    []File   `json:"files"`
	Tags     []string `json:"tags"`
	Address  *address `json:"address"`
	Point    address  `json:"point"`
	Count    int      `json:"count"`
	hidden   string   //nolint:unused // left alone by the decoder
}

type point struct {
	X int `json:"x"`
}

// vertex stands in for a generated union of a string and an object.
type vertex struct {
	Text  *string
	Point *point
}

func (v vertex) MarshalJSON() ([]byte, error) {
	if v.Point != nil {
		return json.Marshal(v.Point)
	}
	return json.Marshal(v.Text)
}

func (v *vertex) UnmarshalJSON(data []byte) error {
	*v = vertex{}
	return UnmarshalUnion(data, Union{Variants: []Variant{
		{Name: "Text", Kind: KindString, Into: Into(&v.Text)},
		{Name: "Point", Kind: KindObject, Required: []string{"x"}, Known: []string{"x"}, Into: Into(&v.Point)},
	}})
}

type drawing struct {
	Vertex   *vertex           `json:"vertex,omitempty"`
	Vertices []vertex          `json:"vertices,omitempty"`
	Named    map[string]vertex `json:"named,omitempty"`
	Origin   *point            `json:"origin,omitempty"`
	Count    int               `json:"count,omitempty"`
}

type blob struct {
	Data  []byte    `json:"data"`
	Opt   *[]byte   `json:"opt"`
	List  [][]byte  `json:"list"`
	Lines []address `json:"lines"`
	Tags  []string  `json:"tags"`
}

// parcel is a form body with a property for each kind of declared media type.
type parcel struct {
	ID    *string   `json:"id,omitempty"`
	Doc   *File     `json:"doc,omitempty"`
	Logo  *File     `json:"logo,omitempty"`
	Pet   *address  `json:"pet,omitempty"`
	Pets  []address `json:"pets,omitempty"`
	Tags  []string  `json:"tags,omitempty"`
	Note  string    `json:"note,omitempty"`
	Plain []address `json:"plain,omitempty"`
}

// styledForm holds a property of each shape a style writes.
type styledForm struct {
	Tags   []string       `json:"tags,omitempty"`
	Color  *rgb           `json:"color,omitempty"`
	Filter map[string]any `json:"filter,omitempty"`
	Note   string         `json:"note,omitempty"`
	Lines  []address      `json:"lines,omitempty"`
}

// scores stands in for a generated struct with integer additional properties that reads forms.
type scores struct {
	Name  string         `json:"name,omitempty"`
	Extra map[string]int `json:"-"`
}

func (s *scores) UnmarshalForm(form *multipart.Form) error {
	type plain scores
	return UnmarshalAdditionalForm(form, (*plain)(s), &s.Extra, "name")
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestContentType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "application/json", ContentType(http.Header{"Content-Type": {"application/json; charset=utf-8"}}))
	assert.Empty(t, ContentType(http.Header{"Content-Type": {"not/valid/type;;"}}))
	assert.Equal(t, "text/event-stream", ContentType(http.Header{"Content-Type": {"text/event-stream; charset"}}), "a parameter without a value")
	assert.True(t, IsJSON("application/vnd.api+json"))
	assert.True(t, IsJSON("Application/JSON; charset=utf-8"))
	assert.False(t, IsJSON("text/plain"))
}

func TestIsSequential(t *testing.T) {
	t.Parallel()

	for _, mt := range []string{"text/event-stream", "application/x-ndjson", "application/ndjson", "application/jsonl", "application/x-jsonlines", "application/json-lines"} {
		assert.True(t, IsSequential(mt), mt)
	}
	assert.True(t, IsSequential("Text/Event-Stream; charset=utf-8"))
	assert.False(t, IsSequential("application/json"))
	assert.False(t, IsSequential("application/stream+json"))
}

func TestMediaRange(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "text/*", MediaRange("text/csv"))
	assert.Equal(t, "image/*", MediaRange("image/png"))
}

func TestContentTypeError(t *testing.T) {
	t.Parallel()

	err := ContentTypeError("text/csv")

	require.ErrorIs(t, err, ErrContentType)
	assert.EqualError(t, err, "unsupported content type: text/csv")
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       io.Reader
		isRequired bool
		want       address
		wantErr    error
	}{
		{name: "Object", body: strings.NewReader(`{"city":"Berlin"}`), want: address{City: "Berlin"}},
		{name: "Empty optional body", body: strings.NewReader(" \n")},
		{name: "Empty required body", body: strings.NewReader(""), isRequired: true, wantErr: ErrBodyEmpty},
		{name: "Broken JSON", body: strings.NewReader("{"), wantErr: io.ErrUnexpectedEOF},
		{name: "Broken reader", body: errReader{}, wantErr: io.ErrUnexpectedEOF},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got address
			err := DecodeJSON(tc.body, &got, tc.isRequired, json.Unmarshal)

			if tc.wantErr != nil {
				require.Error(t, err)
				if errors.Is(tc.wantErr, ErrBodyEmpty) {
					require.ErrorIs(t, err, ErrBodyEmpty)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		isRequired bool
		want       order
		wantErr    error
	}{
		{
			name: "Flat and nested keys",
			body: "name=Ann&age=30&active=true&address[city]=Berlin&address[country]=DE&items[0]=a&items[1]=b&codes[]=1&codes[]=2&meta[x]=true&ignored=1",
			want: order{Name: "Ann", Age: new(30), Active: true, Address: address{City: "Berlin", Country: "DE"}, Items: []string{"a", "b"}, Codes: []int{1, 2}, Meta: map[string]bool{"x": true}},
		},
		{
			name: "Objects in a list",
			body: "lines[0][city]=A&lines[1][city]=B",
			want: order{Lines: []address{{City: "A"}, {City: "B"}}},
		},
		{
			name: "Untyped values are read conservatively",
			body: "extra[n]=42&extra[f]=1.5&extra[b]=false&extra[zeros]=007&extra[phone]=%2B123&extra[text]=a+b&extra[list][]=1&extra[list][]=x&extra[one][]=x&extra[obj][k]=v&extra[big]=99999999999999999999",
			want: order{Extra: map[string]any{
				"n": int64(42), "f": 1.5, "b": false, "zeros": "007", "phone": "+123", "text": "a b",
				"list": []any{int64(1), "x"}, "one": []any{"x"}, "obj": map[string]any{"k": "v"}, "big": "99999999999999999999",
			}},
		},
		{name: "Phone number stays a string", body: "phone=%2B4930", want: order{Phone: "+4930"}},
		{name: "Empty optional body"},
		{name: "Empty required body", isRequired: true, wantErr: ErrBodyEmpty},
		{name: "Bad number", body: "age=x", wantErr: ErrBodyValue},
		{name: "Bad query", body: "a=%zz", wantErr: url.EscapeError("%zz")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got order
			err := DecodeForm(strings.NewReader(tc.body), &got, tc.isRequired, nil)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeFormStyles(t *testing.T) {
	t.Parallel()

	deep := Encoding{"tags": {Style: StyleDeepObject}, "color": {Style: StyleDeepObject}}
	tests := []struct {
		name    string
		body    string
		enc     Encoding
		want    styledForm
		wantErr string
	}{
		{name: "A deep object list under empty brackets", body: "tags[]=a&tags[]=b", enc: deep, want: styledForm{Tags: []string{"a", "b"}}},
		{name: "A deep object list under its bare name", body: "tags=a&tags=b", enc: deep, want: styledForm{Tags: []string{"a", "b"}}},
		{name: "A deep object list of one under its bare name", body: "tags=a", enc: deep, want: styledForm{Tags: []string{"a"}}},
		{name: "A list split before it is read", body: "tags=a,b%2Cc", enc: Encoding{"tags": {Style: StyleForm}}, want: styledForm{Tags: []string{"a", "b,c"}}},
		{name: "An exploded form object takes the keys no property names", body: "R=1&G=2&note=x", enc: Encoding{"color": {Style: StyleForm, IsExplode: true}}, want: styledForm{Color: &rgb{R: 1, G: 2}, Note: "x"}},
		{name: "A value of the wrong type", body: "color[R]=x", enc: deep, wantErr: `invalid body value: color: R: "x" is no int`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got styledForm
			err := DecodeForm(strings.NewReader(tc.body), &got, true, tc.enc)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	var hidden struct {
		tags []string
		Tags []string `json:"tags"`
	}
	require.NoError(t, DecodeForm(strings.NewReader("tags[0]=a"), &hidden, true, deep))
	assert.Equal(t, []string{"a"}, hidden.Tags)
	var m map[string]any
	require.NoError(t, DecodeForm(strings.NewReader("tags[0]=a"), &m, true, deep))
	assert.Equal(t, map[string]any{"tags": []any{"a"}}, m, "a map is no struct to read styles into")
}

func TestDecodeFormValueErrors(t *testing.T) {
	t.Parallel()

	badCount := multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("count", "x"))
	})
	tests := []struct {
		name   string
		decode func(dst any) error
		dst    any
		want   string
	}{
		{
			name:   "A form body names the field",
			decode: func(dst any) error { return DecodeForm(strings.NewReader("age=x"), dst, false, nil) },
			dst:    new(order),
			want:   `invalid body value: age: "x" is no int`,
		},
		{
			name:   "A form body names the nested field",
			decode: func(dst any) error { return DecodeForm(strings.NewReader("meta[on]=maybe"), dst, false, nil) },
			dst:    new(order),
			want:   `invalid body value: meta: on: "maybe" is no bool`,
		},
		{
			name:   "A multipart body names the field",
			decode: func(dst any) error { return DecodeMultipart(badCount, dst, 0, nil) },
			dst:    new(upload),
			want:   `invalid body value: count: "x" is no int`,
		},
		{
			name:   "A form body that reads itself is a body",
			decode: func(dst any) error { return DecodeForm(strings.NewReader("n=x"), dst, false, nil) },
			dst:    new(scores),
			want:   `invalid body value: invalid additional property "n": "x" is no int`,
		},
		{
			name:   "The same form as a query string is a parameter",
			decode: func(dst any) error { return DecodeQueryString("n=x", Param{Name: "q"}, dst) },
			dst:    new(scores),
			want:   `invalid parameter value: invalid additional property "n": "x" is no int`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.EqualError(t, tc.decode(tc.dst), tc.want)
		})
	}
}

func TestDecodeFormEdges(t *testing.T) {
	t.Parallel()

	require.ErrorIs(t, DecodeForm(errReader{}, new(order), false, nil), io.ErrUnexpectedEOF)
	require.ErrorIs(t, DecodeForm(strings.NewReader("a=1"), order{}, false, nil), ErrParamValue)
}

func TestDecodeFormText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want drawing
	}{
		{name: "Plain text is the string", body: "vertex=abc", want: drawing{Vertex: &vertex{Text: new("abc")}}},
		{name: "A number no variant takes is the string", body: "vertex=12", want: drawing{Vertex: &vertex{Text: new("12")}}},
		{name: "A JSON string", body: "vertex=%22q%22", want: drawing{Vertex: &vertex{Text: new("q")}}},
		{name: "A JSON object", body: "vertex=%7B%22x%22%3A7%7D", want: drawing{Vertex: &vertex{Point: &point{X: 7}}}},
		{name: "Items and map values", body: "vertices=a&vertices=%7B%22x%22%3A1%7D&named[n]=b", want: drawing{
			Vertices: []vertex{{Text: new("a")}, {Point: &point{X: 1}}},
			Named:    map[string]vertex{"n": {Text: new("b")}},
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got drawing
			require.NoError(t, DecodeForm(strings.NewReader(tc.body), &got, false, nil))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeFormTextErrors(t *testing.T) {
	t.Parallel()

	for _, body := range []string{"origin=12", "origin=nope", "origin=%7B%22x%22%3A%22a%22%7D"} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, DecodeForm(strings.NewReader(body), new(drawing), false, nil), ErrBodyValue)
		})
	}
}

func TestDecodeFormBytesAndJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		want    blob
		wantErr error
	}{
		{
			name: "Bytes are base64",
			body: "data=YWJj&opt=eHl6&list=YQ%3D%3D&list=Yg%3D%3D",
			want: blob{Data: []byte("abc"), Opt: new([]byte("xyz")), List: [][]byte{[]byte("a"), []byte("b")}},
		},
		{name: "Text that is no base64", body: "data=abc", wantErr: ErrBodyValue},
		{name: "A JSON array fills a list of objects", body: "lines=%5B%7B%22city%22%3A%22A%22%7D%5D", want: blob{Lines: []address{{City: "A"}}}},
		{name: "Text in brackets that is no JSON is text", body: "tags=%5Bdraft%5D", want: blob{Tags: []string{"[draft]"}}},
		{name: "A key that names nothing is left out", body: "=x&data=YQ%3D%3D", want: blob{Data: []byte("a")}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got blob
			err := DecodeForm(strings.NewReader(tc.body), &got, false, nil)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeFormUnmarshaler(t *testing.T) {
	t.Parallel()

	var got *post
	require.NoError(t, DecodeForm(strings.NewReader("id=1&name=Tom&meow=true"), &got, true, nil))
	assert.Equal(t, &post{ID: "1", Cat: &cat{Name: "Tom", Meow: true}}, got)

	var feed struct {
		Post  *post  `json:"post"`
		Posts []post `json:"posts"`
	}
	body := "post[meow]=false&posts[0][image]=x&posts[0][caption]=c&posts[0][tags][0]=a&posts[0][tags][1]=b"
	require.NoError(t, DecodeForm(strings.NewReader(body), &feed, true, nil))
	assert.Equal(t, &post{Cat: &cat{}}, feed.Post)
	assert.Equal(t, []post{{Photo: &photo{Caption: "c", Tags: []string{"a", "b"}}}}, feed.Posts)
}

func multipartRequest(t *testing.T, write func(w *multipart.Writer)) *http.Request {
	t.Helper()

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	write(w)
	require.NoError(t, w.Close())
	r := httptest.NewRequest(http.MethodPost, "/", &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}

func TestDecodeMultipart(t *testing.T) {
	t.Parallel()

	r := multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("title", "Report"))
		require.NoError(t, w.WriteField("tags", "a"))
		require.NoError(t, w.WriteField("tags", "b"))
		require.NoError(t, w.WriteField("address", `{"city":"Berlin"}`))
		require.NoError(t, w.WriteField("point[city]", "Paris"))
		require.NoError(t, w.WriteField("count", "3"))
		for _, name := range []string{"file", "optional", "files", "files"} {
			part, err := w.CreateFormFile(name, name+".txt")
			require.NoError(t, err)
			_, err = part.Write([]byte("content of " + name))
			require.NoError(t, err)
		}
	})

	var got upload
	require.NoError(t, DecodeMultipart(r, &got, 0, nil))

	assert.Equal(t, "Report", got.Title)
	assert.Equal(t, []string{"a", "b"}, got.Tags)
	assert.Equal(t, &address{City: "Berlin"}, got.Address)
	assert.Equal(t, address{City: "Paris"}, got.Point)
	assert.Equal(t, 3, got.Count)
	assert.Equal(t, "file.txt", got.File.Name())
	data, err := got.File.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "content of file", string(data))
	require.NotNil(t, got.Optional)
	assert.Equal(t, "optional.txt", got.Optional.Name())
	assert.Len(t, got.Files, 2)
}

func TestDecodeMultipartBytes(t *testing.T) {
	t.Parallel()

	r := multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("data", "YWJj"))
		part, err := w.CreateFormFile("opt", "raw.bin")
		require.NoError(t, err)
		_, err = part.Write([]byte{0, 1})
		require.NoError(t, err)
	})

	var got blob
	require.NoError(t, DecodeMultipart(r, &got, 0, nil))
	assert.Equal(t, blob{Data: []byte("abc"), Opt: new([]byte{0, 1})}, got)

	_, err := setFiles(reflect.ValueOf(&got.Data).Elem(), []*multipart.FileHeader{{Filename: "gone"}})
	require.Error(t, err)
}

func TestDecodeMultipartUnmarshaler(t *testing.T) {
	t.Parallel()

	r := multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("id", "9"))
		part, err := w.CreateFormFile("image", "a.png")
		require.NoError(t, err)
		_, err = part.Write([]byte("PNG"))
		require.NoError(t, err)
	})

	var got post
	require.NoError(t, DecodeMultipart(r, &got, 0, nil))
	assert.Equal(t, "9", got.ID)
	require.NotNil(t, got.Photo)
	assert.Equal(t, "a.png", got.Photo.Image.Name())
}

func TestDecodeMultipartEdges(t *testing.T) {
	t.Parallel()

	empty := multipartRequest(t, func(*multipart.Writer) {})
	var got upload
	require.NoError(t, DecodeMultipart(empty, &got, 1, nil))
	assert.Equal(t, upload{}, got)

	require.ErrorIs(t, DecodeMultipart(multipartRequest(t, func(*multipart.Writer) {}), new(int), 0, nil), ErrBodyValue)
	require.ErrorIs(t, DecodeMultipart(multipartRequest(t, func(*multipart.Writer) {}), 1, 0, nil), ErrParamValue)
	require.ErrorIs(t, DecodeMultipart(multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("count", "x"))
	}), &got, 0, nil), ErrBodyValue)
	require.Error(t, DecodeMultipart(multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("address", "{"))
	}), &got, 0, nil))

	plain := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("a=1"))
	plain.Header.Set("Content-Type", "text/plain")
	require.Error(t, DecodeMultipart(plain, &got, 0, nil))
}

func TestDecodeMultipartBody(t *testing.T) {
	t.Parallel()

	r := multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("title", "Report"))
		part, err := w.CreateFormFile("file", "a.txt")
		require.NoError(t, err)
		_, err = part.Write([]byte("hi"))
		require.NoError(t, err)
	})
	contentType := r.Header.Get("Content-Type")
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	var got *upload
	require.NoError(t, DecodeMultipartBody(bytes.NewReader(body), contentType, &got, nil))
	require.NotNil(t, got)
	assert.Equal(t, "Report", got.Title)
	data, err := got.File.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "hi", string(data))

	tests := []struct {
		name        string
		body        string
		contentType string
		dst         any
		wantErr     error
	}{
		{name: "A content type that does not parse", contentType: "multipart/form-data; boundary", dst: new(upload), wantErr: mime.ErrInvalidMediaParameter},
		{name: "No boundary", contentType: "multipart/form-data", dst: new(upload), wantErr: http.ErrMissingBoundary},
		{name: "A body cut short", body: "--b\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\nx", contentType: "multipart/form-data; boundary=b", dst: new(upload), wantErr: io.ErrUnexpectedEOF},
		{name: "No struct", body: string(body), contentType: contentType, dst: new(int), wantErr: ErrBodyValue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, DecodeMultipartBody(strings.NewReader(tc.body), tc.contentType, tc.dst, nil), tc.wantErr)
		})
	}
}

func TestDecodeTextAndBytes(t *testing.T) {
	t.Parallel()

	text, err := DecodeText(strings.NewReader("hi"), true)
	require.NoError(t, err)
	assert.Equal(t, "hi", text)
	_, err = DecodeText(strings.NewReader(""), true)
	require.ErrorIs(t, err, ErrBodyEmpty)
	_, err = DecodeText(errReader{}, false)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)

	data, err := DecodeBytes(strings.NewReader("hi"), false)
	require.NoError(t, err)
	assert.Equal(t, []byte("hi"), data)
	data, err = DecodeBytes(strings.NewReader(""), false)
	require.NoError(t, err)
	assert.Nil(t, data)
	_, err = DecodeBytes(strings.NewReader(""), true)
	require.ErrorIs(t, err, ErrBodyEmpty)
}

func TestDecodeTextValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		dst        any
		isRequired bool
		want       any
		wantErr    error
	}{
		{name: "A number", body: "3", dst: new(*int), want: new(new(3))},
		{name: "A boolean", body: "true", dst: new(bool), want: new(true)},
		{name: "A time by its text", body: "2026-01-02T03:04:05Z", dst: new(time.Time), want: new(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))},
		{name: "Any holds the text", body: "3", dst: new(any), want: new(any("3"))},
		{name: "An empty body leaves the value", dst: new(int), want: new(0)},
		{name: "An empty required body", dst: new(int), isRequired: true, wantErr: ErrBodyEmpty},
		{name: "Text that is no number", body: "three", dst: new(int), wantErr: ErrBodyValue},
		{name: "No pointer", body: "3", dst: 3, wantErr: ErrParamValue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := DecodeTextValue(strings.NewReader(tc.body), tc.dst, tc.isRequired)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, tc.dst)
		})
	}
}

func TestDecodeFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       io.Reader
		isRequired bool
		want       string
		wantType   string
		wantSize   int64
		wantErr    error
	}{
		{name: "A body streams under its media type", body: strings.NewReader("png"), want: "png", wantType: "image/png", wantSize: 3},
		{name: "An empty optional body", body: strings.NewReader("")},
		{name: "An empty required body", body: strings.NewReader(""), isRequired: true, wantErr: ErrBodyEmpty},
		{name: "A broken body", body: errReader{}, wantErr: io.ErrUnexpectedEOF},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodPut, "/", tc.body)
			r.Header.Set("Content-Type", "Image/PNG; q=1")

			f, err := DecodeFile(r, tc.isRequired)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			data, err := f.Bytes()
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(data))
			assert.Equal(t, tc.wantType, f.ContentType())
			assert.Equal(t, tc.wantSize, f.Size())
		})
	}
}

func TestDecodeEncoding(t *testing.T) {
	t.Parallel()

	enc := Encoding{"id": {ContentType: "application/json"}, "pet": {ContentType: "application/json"}, "pets": {ContentType: "application/json"}, "tags": {ContentType: "application/json"}}
	tests := []struct {
		name    string
		body    string
		want    parcel
		wantErr string
	}{
		{name: "A JSON string", body: `id="p1"`, want: parcel{ID: Ptr("p1")}},
		{name: "JSON null", body: `id=null`, want: parcel{}},
		{name: "A list whole", body: `pets=[{"city":"A"},{"city":"B"}]`, want: parcel{Pets: []address{{City: "A"}, {City: "B"}}}},
		{name: "A list item by item", body: `pets={"city":"A"}&pets={"city":"B"}`, want: parcel{Pets: []address{{City: "A"}, {City: "B"}}}},
		{name: "A list of JSON strings", body: `tags="a"&tags=["b","c"]`, want: parcel{Tags: []string{"a", "b", "c"}}},
		{name: "An object with brackets", body: `pet[city]=Rome`, want: parcel{Pet: &address{City: "Rome"}}},
		{name: "Text that is no JSON", body: `id=p1`, wantErr: "invalid body value: id: invalid character 'p' looking for beginning of value"},
		{name: "An item that is no JSON", body: `pets={"city":"A"}&pets=x`, wantErr: "invalid body value: pets: invalid character 'x' looking for beginning of value"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got parcel
			err := DecodeForm(strings.NewReader(tc.body), &got, false, enc)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeMultipartFilePartAsText(t *testing.T) {
	t.Parallel()

	r := multipartRequest(t, func(w *multipart.Writer) {
		fw, err := w.CreateFormFile("pet", "blob")
		require.NoError(t, err)
		_, err = fw.Write([]byte(`{"city":"Rome"}`))
		require.NoError(t, err)
		fw, err = w.CreateFormFile("note", "note.txt")
		require.NoError(t, err)
		_, err = fw.Write([]byte("hi"))
		require.NoError(t, err)
	})

	var got parcel
	require.NoError(t, DecodeMultipart(r, &got, 0, Encoding{"pet": {ContentType: "application/json"}}))
	assert.Equal(t, parcel{Pet: &address{City: "Rome"}, Note: "hi"}, got)

	r = multipartRequest(t, func(w *multipart.Writer) {
		_, err := w.CreateFormFile("note", "gone")
		require.NoError(t, err)
	})
	require.NoError(t, r.ParseMultipartForm(1<<20))
	r.MultipartForm.File["note"][0] = &multipart.FileHeader{Filename: "gone"}
	require.Error(t, DecodeMultipart(r, &got, 0, nil))
}
