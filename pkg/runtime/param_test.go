// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rgb struct {
	R int `json:"R"`
	G int `json:"G"`
	B int `json:"B"`
}

type filter struct {
	Name   *string           `json:"name,omitempty"`
	Tags   []string          `json:"tags,omitempty"`
	Size   *rgb              `json:"size,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

var (
	colors = []string{"blue", "black", "brown"}
	color  = rgb{R: 100, G: 200, B: 150}
)

func explode(s Style, isExplode bool) Param {
	return Param{Name: "color", Style: s, IsExplode: isExplode}
}

func TestUnescapePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		path  string
		value string
		want  string
	}{
		{name: "A value cut from the raw path", path: "/users/john%40example.com", value: "john%40example.com", want: "john@example.com"},
		{name: "An escaped slash", path: "/users/a%2Fb", value: "a%2Fb", want: "a/b"},
		{name: "Without a raw path the value is unescaped already", path: "/users/100%25", value: "100%", want: "100%"},
		{name: "A value that does not unescape", path: "/users/a%2Fb", value: "a%zz", want: "a%zz"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, tc.path, nil)

			assert.Equal(t, tc.want, UnescapePath(r, tc.value))
		})
	}
}

func TestPathStyles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		param Param
		value any
		text  string
	}{
		{name: "Simple string", param: explode(StyleSimple, false), value: "blue", text: "blue"},
		{name: "Simple list", param: explode(StyleSimple, false), value: colors, text: "blue,black,brown"},
		{name: "Simple object", param: explode(StyleSimple, false), value: color, text: "R,100,G,200,B,150"},
		{name: "Simple exploded list", param: explode(StyleSimple, true), value: colors, text: "blue,black,brown"},
		{name: "Simple exploded object", param: explode(StyleSimple, true), value: color, text: "R=100,G=200,B=150"},
		{name: "Label string", param: explode(StyleLabel, false), value: "blue", text: ".blue"},
		{name: "Label list", param: explode(StyleLabel, false), value: colors, text: ".blue,black,brown"},
		{name: "Label object", param: explode(StyleLabel, false), value: color, text: ".R,100,G,200,B,150"},
		{name: "Label exploded list", param: explode(StyleLabel, true), value: colors, text: ".blue.black.brown"},
		{name: "Label exploded object", param: explode(StyleLabel, true), value: color, text: ".R=100.G=200.B=150"},
		{name: "Matrix string", param: explode(StyleMatrix, false), value: "blue", text: ";color=blue"},
		{name: "Matrix list", param: explode(StyleMatrix, false), value: colors, text: ";color=blue,black,brown"},
		{name: "Matrix object", param: explode(StyleMatrix, false), value: color, text: ";color=R,100,G,200,B,150"},
		{name: "Matrix exploded string", param: explode(StyleMatrix, true), value: "blue", text: ";color=blue"},
		{name: "Matrix exploded list", param: explode(StyleMatrix, true), value: colors, text: ";color=blue;color=black;color=brown"},
		{name: "Matrix exploded object", param: explode(StyleMatrix, true), value: color, text: ";R=100;G=200;B=150"},
		{name: "Number", param: explode(StyleSimple, false), value: 42, text: "42"},
		{name: "Time", param: explode(StyleSimple, false), value: time.Date(2026, 9, 30, 1, 2, 3, 0, time.UTC), text: "2026-09-30T01:02:03Z"},
		{name: "JSON", param: Param{Name: "color", IsJSON: true}, value: color, text: `{"R":100,"G":200,"B":150}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := EncodePath(tc.value, tc.param)
			require.NoError(t, err)
			assert.Equal(t, tc.text, got)

			dst := reflect.New(reflect.TypeOf(tc.value))
			require.NoError(t, DecodePath(tc.text, tc.param, dst.Interface()))
			assert.Equal(t, tc.value, dst.Elem().Interface())
		})
	}
}

func TestParseQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want Query
	}{
		{name: "Keys are unescaped and values kept", raw: "a%5B0%5D=x%2Cy&b=1&b=2", want: Query{"a[0]": {"x%2Cy"}, "b": {"1", "2"}}},
		{name: "A key without a value and an empty item", raw: "a&&b=", want: Query{"a": {""}, "b": {""}}},
		{name: "A key that does not unescape is left out", raw: "%zz=1&a=2", want: Query{"a": {"2"}}},
		{name: "A semicolon is part of a value", raw: "a=1;2", want: Query{"a": {"1;2"}}},
		{name: "No query", raw: "", want: Query{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, ParseQuery(tc.raw))
		})
	}
}

func TestQueryStyles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		param Param
		value any
		text  string
	}{
		{name: "Form string", param: explode(StyleForm, false), value: "blue", text: "color=blue"},
		{name: "Form list", param: explode(StyleForm, false), value: colors, text: "color=blue,black,brown"},
		{name: "Form object", param: explode(StyleForm, false), value: color, text: "color=R,100,G,200,B,150"},
		{name: "Form exploded list", param: explode(StyleForm, true), value: colors, text: "color=blue&color=black&color=brown"},
		{name: "Form exploded object", param: explode(StyleForm, true), value: color, text: "R=100&G=200&B=150"},
		{name: "Space delimited list", param: explode(StyleSpaceDelimited, false), value: colors, text: "color=blue%20black%20brown"},
		{name: "Space delimited object", param: explode(StyleSpaceDelimited, false), value: color, text: "color=R%20100%20G%20200%20B%20150"},
		{name: "Pipe delimited list", param: explode(StylePipeDelimited, false), value: colors, text: "color=blue%7Cblack%7Cbrown"},
		{name: "Pipe delimited object", param: explode(StylePipeDelimited, false), value: color, text: "color=R%7C100%7CG%7C200%7CB%7C150"},
		{name: "Deep object", param: explode(StyleDeepObject, true), value: color, text: "color%5BR%5D=100&color%5BG%5D=200&color%5BB%5D=150"},
		{name: "Deep object into a map", param: explode(StyleDeepObject, true), value: map[string]string{"R": "100"}, text: "color%5BR%5D=100"},
		{
			name:  "Deep object with a list, an object and a map inside",
			param: explode(StyleDeepObject, false),
			value: filter{Name: new("a"), Tags: []string{"x", "y"}, Size: &rgb{R: 1}, Labels: map[string]string{"k": "v"}},
			text: "color%5Bname%5D=a&color%5Btags%5D=x&color%5Btags%5D=y&color%5Bsize%5D%5BR%5D=1&color%5Bsize%5D%5BG%5D=0" +
				"&color%5Bsize%5D%5BB%5D=0&color%5Blabels%5D%5Bk%5D=v",
		},
		{name: "Deep object with a list of one", param: explode(StyleDeepObject, false), value: filter{Tags: []string{"x"}}, text: "color%5Btags%5D=x"},
		{name: "Form object with its list unset", param: explode(StyleForm, false), value: filter{Name: new("a")}, text: "color=name,a"},
		{name: "JSON", param: Param{Name: "color", IsJSON: true}, value: color, text: "color=%7B%22R%22%3A100%2C%22G%22%3A200%2C%22B%22%3A150%7D"},
		{name: "A space is %20 and a plus escaped", param: explode(StyleForm, true), value: "name eq 'a+b'", text: "color=name%20eq%20%27a%2Bb%27"},
		{name: "A comma inside an item is escaped, one between items is not", param: explode(StyleForm, false), value: []string{"a", "b,c"}, text: "color=a,b%2Cc"},
		{name: "A comma inside a name or value of an object", param: explode(StyleForm, false), value: map[string]string{"a,b": "c,d"}, text: "color=a%2Cb,c%2Cd"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := EncodeQuery(tc.value, tc.param)
			require.NoError(t, err)
			assert.Equal(t, tc.text, got)

			dst := reflect.New(reflect.TypeOf(tc.value))
			require.NoError(t, DecodeQuery(ParseQuery(tc.text), tc.param, dst.Interface()))
			assert.Equal(t, tc.value, dst.Elem().Interface())
		})
	}
}

func TestEncodeQueryReserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		param Param
		value any
		want  string
	}{
		{name: "Reserved characters go as they are", param: Param{Name: "ids", Style: StyleForm, IsReserved: true}, value: "List(1,2)", want: "ids=List(1,2)"},
		{name: "Each one a query holds", param: Param{Name: "q", Style: StyleForm, IsReserved: true}, value: "!$&'()*+,;=:@/?", want: "q=!$&'()*+,;=:@/?"},
		{name: "A query holds no brackets or hash", param: Param{Name: "q", Style: StyleForm, IsReserved: true}, value: "a[0]#b", want: "q=a%5B0%5D%23b"},
		{name: "An escape stays, a lone percent does not", param: Param{Name: "q", Style: StyleForm, IsReserved: true}, value: "a%2Fb%2f%zz%4", want: "q=a%2Fb%2f%25zz%254"},
		{name: "A space is still escaped", param: Param{Name: "q", Style: StyleForm, IsReserved: true}, value: "a b", want: "q=a%20b"},
		{name: "Items keep their commas", param: Param{Name: "q", Style: StyleForm, IsReserved: true}, value: []string{"a,b", "c"}, want: "q=a,b,c"},
		{name: "The name is escaped as ever", param: Param{Name: "a(b", Style: StyleForm, IsExplode: true, IsReserved: true}, value: "(", want: "a%28b=("},
		{name: "Without it reserved characters are escaped", param: Param{Name: "ids", Style: StyleForm}, value: "List(1,2)", want: "ids=List%281%2C2%29"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := EncodeQuery(tc.value, tc.param)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHeaderStyles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		param Param
		value any
		text  string
	}{
		{name: "String", param: explode(StyleSimple, false), value: "blue", text: "blue"},
		{name: "List", param: explode(StyleSimple, false), value: colors, text: "blue,black,brown"},
		{name: "Object", param: explode(StyleSimple, false), value: color, text: "R,100,G,200,B,150"},
		{name: "Exploded object", param: explode(StyleSimple, true), value: color, text: "R=100,G=200,B=150"},
		{name: "JSON", param: Param{Name: "color", IsJSON: true}, value: colors, text: `["blue","black","brown"]`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := EncodeHeader(tc.value, tc.param)
			require.NoError(t, err)
			assert.Equal(t, tc.text, got)

			dst := reflect.New(reflect.TypeOf(tc.value))
			require.NoError(t, DecodeHeader(http.Header{"Color": []string{tc.text}}, tc.param, dst.Interface()))
			assert.Equal(t, tc.value, dst.Elem().Interface())
		})
	}
}

func TestCookieStyles(t *testing.T) {
	t.Parallel()

	cookie := func(name, value string) *http.Cookie { return &http.Cookie{Name: name, Value: value} }
	tests := []struct {
		name    string
		param   Param
		value   any
		cookies []*http.Cookie
	}{
		{name: "String", param: explode(StyleForm, false), value: "blue", cookies: []*http.Cookie{cookie("color", "blue")}},
		{name: "List", param: explode(StyleForm, false), value: colors, cookies: []*http.Cookie{cookie("color", "blue,black,brown")}},
		{name: "Object", param: explode(StyleForm, false), value: color, cookies: []*http.Cookie{cookie("color", "R,100,G,200,B,150")}},
		{name: "Exploded list", param: explode(StyleForm, true), value: colors, cookies: []*http.Cookie{cookie("color", "blue"), cookie("color", "black"), cookie("color", "brown")}},
		{name: "Exploded object", param: explode(StyleForm, true), value: color, cookies: []*http.Cookie{cookie("R", "100"), cookie("G", "200"), cookie("B", "150")}},
		{name: "JSON", param: Param{Name: "color", IsJSON: true}, value: colors, cookies: []*http.Cookie{cookie("color", `["blue","black","brown"]`)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := EncodeCookie(tc.value, tc.param)
			require.NoError(t, err)
			assert.Equal(t, tc.cookies, got)

			dst := reflect.New(reflect.TypeOf(tc.value))
			require.NoError(t, DecodeCookie(tc.cookies, tc.param, dst.Interface()))
			assert.Equal(t, tc.value, dst.Elem().Interface())
		})
	}
}

func TestDecodeParamEdges(t *testing.T) {
	t.Parallel()

	optional, required := Param{Name: "color", Style: StyleForm}, Param{Name: "color", Style: StyleForm, IsRequired: true}
	tests := []struct {
		name    string
		decode  func(dst any) error
		dst     any
		want    any
		wantErr error
	}{
		{name: "Missing optional query leaves the target", decode: func(dst any) error { return DecodeQuery(nil, optional, dst) }, dst: new(*string), want: (*string)(nil)},
		{name: "Missing required query", decode: func(dst any) error { return DecodeQuery(nil, required, dst) }, dst: new(string), wantErr: ErrParamMissing},
		{name: "Missing exploded object", decode: func(dst any) error {
			return DecodeQuery(nil, Param{Name: "c", Style: StyleForm, IsExplode: true, IsRequired: true}, dst)
		}, dst: new(rgb), wantErr: ErrParamMissing},
		{name: "Missing deep object", decode: func(dst any) error { return DecodeQuery(Query{"x": {"1"}}, explode(StyleDeepObject, true), dst) }, dst: new(rgb), want: rgb{}},
		{name: "Missing header", decode: func(dst any) error { return DecodeHeader(http.Header{}, required, dst) }, dst: new(string), wantErr: ErrParamMissing},
		{name: "Missing cookie", decode: func(dst any) error { return DecodeCookie(nil, required, dst) }, dst: new(string), wantErr: ErrParamMissing},
		{name: "Bad number", decode: func(dst any) error { return DecodePath("x", explode(StyleSimple, false), dst) }, dst: new(int), wantErr: ErrParamValue},
		{name: "Bad JSON", decode: func(dst any) error { return DecodePath("{", Param{IsJSON: true}, dst) }, dst: new(rgb), wantErr: ErrParamValue},
		{name: "Target that is no pointer", decode: func(dst any) error { return DecodePath("1", optional, dst) }, dst: 1, wantErr: ErrParamValue},
		{name: "Pointer target is allocated", decode: func(dst any) error { return DecodePath("blue", optional, dst) }, dst: new(*string), want: new("blue")},
		{name: "Nested deep object", decode: func(dst any) error {
			return DecodeQuery(Query{"color[a][b]": {"1"}, "color[a][c]": {"x", "y"}}, explode(StyleDeepObject, true), dst)
		}, dst: new(map[string]any), want: map[string]any{"a": map[string]any{"b": "1", "c": []any{"x", "y"}}}},
		{name: "Pipes inside a header", decode: func(dst any) error { return DecodeHeader(http.Header{"Color": {"a", "b"}}, optional, dst) }, dst: new([]string), want: []string{"a", "b"}},
		{name: "Cookie object exploded", decode: func(dst any) error {
			return DecodeCookie([]*http.Cookie{{Name: "R", Value: "1"}}, explode(StyleForm, true), dst)
		}, dst: new(rgb), want: rgb{R: 1}},
		{name: "Missing query takes its default", decode: func(dst any) error {
			return DecodeQuery(nil, Param{Name: "color", Style: StyleForm, Default: `["red"]`}, dst)
		}, dst: new([]string), want: []string{"red"}},
		{name: "Missing required query ignores the default", decode: func(dst any) error {
			return DecodeQuery(nil, Param{Name: "color", IsRequired: true, Default: `"red"`}, dst)
		}, dst: new(string), wantErr: ErrParamMissing},
		{name: "Missing header takes its default", decode: func(dst any) error {
			return DecodeHeader(http.Header{}, Param{Name: "X-Limit", Default: "20"}, dst)
		}, dst: new(*int), want: new(20)},
		{name: "Querystring form", decode: func(dst any) error {
			return DecodeQueryString("name=rex&tags=a&tags=b", Param{Name: "q"}, dst)
		}, dst: new(filter), want: filter{Name: new("rex"), Tags: []string{"a", "b"}}},
		{name: "Querystring JSON", decode: func(dst any) error {
			return DecodeQueryString("%7B%22name%22%3A%22a+b%22%7D", Param{Name: "q", IsJSON: true}, dst)
		}, dst: new(*filter), want: &filter{Name: new("a+b")}},
		{name: "Missing querystring takes its default", decode: func(dst any) error {
			return DecodeQueryString("", Param{Name: "q", IsJSON: true, Default: `{"name":"all"}`}, dst)
		}, dst: new(filter), want: filter{Name: new("all")}},
		{name: "Missing required querystring", decode: func(dst any) error {
			return DecodeQueryString("", Param{Name: "q", IsRequired: true}, dst)
		}, dst: new(filter), wantErr: ErrParamMissing},
		{name: "Querystring form with a bad escape", decode: func(dst any) error {
			return DecodeQueryString("name=%zz", Param{Name: "q"}, dst)
		}, dst: new(filter), wantErr: ErrParamValue},
		{name: "Querystring JSON with a bad escape", decode: func(dst any) error {
			return DecodeQueryString("%zz", Param{Name: "q", IsJSON: true}, dst)
		}, dst: new(filter), wantErr: ErrParamValue},
		{name: "Querystring target that is no pointer", decode: func(dst any) error {
			return DecodeQueryString("a=1", Param{Name: "q"}, dst)
		}, dst: 1, wantErr: ErrParamValue},
		{name: "Bytes are base64", decode: func(dst any) error {
			return DecodeQuery(Query{"color": {"YWJj"}}, optional, dst)
		}, dst: new([]byte), want: []byte("abc")},
		{name: "Bytes that are no base64", decode: func(dst any) error {
			return DecodeHeader(http.Header{"Color": {"abc"}}, optional, dst)
		}, dst: new([]byte), wantErr: ErrParamValue},
		{name: "A deep object key that names nothing", decode: func(dst any) error {
			return DecodeQuery(Query{"color[": {"1"}, "color[a]": {"2"}}, explode(StyleDeepObject, true), dst)
		}, dst: new(map[string]any), want: map[string]any{"a": "2"}},
		{name: "A deep object key without values is left out", decode: func(dst any) error {
			return DecodeQuery(Query{"color[R]": nil, "color[G]": {"2"}}, explode(StyleDeepObject, true), dst)
		}, dst: new(rgb), want: rgb{G: 2}},
		{name: "An exploded object key without values is left out", decode: func(dst any) error {
			return DecodeQuery(Query{"R": nil, "G": {"2"}}, explode(StyleForm, true), dst)
		}, dst: new(rgb), want: rgb{G: 2}},
		{name: "A plus in a query value is a space", decode: func(dst any) error {
			return DecodeQuery(Query{"color": {"a+b%2B"}}, optional, dst)
		}, dst: new(string), want: "a b+"},
		{name: "A space delimited item with an escaped space splits", decode: func(dst any) error {
			return DecodeQuery(Query{"color": {"a+b%20c"}}, explode(StyleSpaceDelimited, false), dst)
		}, dst: new([]string), want: []string{"a", "b", "c"}},
		{name: "A query value that does not unescape", decode: func(dst any) error {
			return DecodeQuery(Query{"color": {"%zz"}}, optional, dst)
		}, dst: new(string), wantErr: ErrParamValue},
		{name: "A list item that does not unescape", decode: func(dst any) error {
			return DecodeQuery(Query{"color": {"a,%zz"}}, optional, dst)
		}, dst: new([]string), wantErr: ErrParamValue},
		{name: "An exploded list item that does not unescape", decode: func(dst any) error {
			return DecodeQuery(Query{"color": {"a", "%zz"}}, explode(StyleForm, true), dst)
		}, dst: new([]string), wantErr: ErrParamValue},
		{name: "A space delimited value that does not unescape", decode: func(dst any) error {
			return DecodeQuery(Query{"color": {"a%zz"}}, explode(StyleSpaceDelimited, false), dst)
		}, dst: new([]string), wantErr: ErrParamValue},
		{name: "A deep object value that does not unescape", decode: func(dst any) error {
			return DecodeQuery(Query{"color[R]": {"%zz"}}, explode(StyleDeepObject, true), dst)
		}, dst: new(rgb), wantErr: ErrParamValue},
		{name: "An exploded object value that does not unescape", decode: func(dst any) error {
			return DecodeQuery(Query{"R": {"%zz"}}, explode(StyleForm, true), dst)
		}, dst: new(rgb), wantErr: ErrParamValue},
		{name: "A JSON query value that does not unescape", decode: func(dst any) error {
			return DecodeQuery(Query{"color": {"%zz"}}, Param{Name: "color", IsJSON: true}, dst)
		}, dst: new(rgb), wantErr: ErrParamValue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.decode(tc.dst)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, reflect.ValueOf(tc.dst).Elem().Interface())
		})
	}
}

func TestEncodeParamEdges(t *testing.T) {
	t.Parallel()

	p := explode(StyleForm, true)
	query, err := EncodeQuery((*string)(nil), p)
	require.NoError(t, err)
	assert.Empty(t, query)

	cookies, err := EncodeCookie((*rgb)(nil), p)
	require.NoError(t, err)
	assert.Nil(t, cookies)

	_, err = EncodePath(make(chan int), p)
	require.ErrorIs(t, err, ErrParamValue)
	_, err = EncodePath([]chan int{nil}, p)
	require.ErrorIs(t, err, ErrParamValue)
	_, err = EncodeHeader(map[string]chan int{"a": nil}, p)
	require.ErrorIs(t, err, ErrParamValue)
	_, err = EncodeQuery(struct{ C chan int }{}, p)
	require.ErrorIs(t, err, ErrParamValue)
	_, err = EncodeQuery(filter{Tags: []string{"x"}}, p)
	require.ErrorIs(t, err, ErrParamValue, "only a deep object writes a list inside")
	deep := explode(StyleDeepObject, false)
	for _, value := range []any{struct{ C chan int }{}, struct{ L []chan int }{L: []chan int{nil}}, struct{ O struct{ C chan int } }{}} {
		_, err = EncodeQuery(value, deep)
		require.ErrorIs(t, err, ErrParamValue)
	}
	query, err = EncodeQuery(filter{Tags: []string{}, Labels: map[string]string{}}, deep)
	require.NoError(t, err)
	assert.Empty(t, query, "an empty list or map is left out")
	_, err = EncodePath(func() {}, Param{IsJSON: true})
	require.Error(t, err)
	require.ErrorIs(t, DecodeHeader(http.Header{}, p, 1), ErrParamValue)
	require.ErrorIs(t, DecodeCookie(nil, p, 1), ErrParamValue)
	require.ErrorIs(t, DecodeQuery(nil, p, 1), ErrParamValue)

	bytesParam, err := EncodePath([]byte("abc"), explode(StyleSimple, false))
	require.NoError(t, err)
	assert.Equal(t, "YWJj", bytesParam)

	for value, want := range map[any]string{true: "true", uint8(3): "3", 1.5: "1.5", (*int)(nil): "", new("x"): "x"} {
		got, encodeErr := EncodePath(value, explode(StyleSimple, false))
		require.NoError(t, encodeErr)
		assert.Equal(t, want, got)
	}

	got, err := EncodePath(struct {
		Skip   *int `json:"-"`
		Nil    *int `json:"nil"`
		Any    any  `json:"any"`
		hidden string
	}{Any: 3, hidden: "x"}, explode(StyleSimple, true))
	require.NoError(t, err)
	assert.Equal(t, "any=3", got)
}

func TestHeaders(t *testing.T) {
	t.Parallel()

	typed := struct {
		Count  int      `json:"X-Total-Count"`
		Token  *string  `json:"X-Page-Token"`
		Tags   []string `json:"X-Tags"`
		Bad    chan int `json:"X-Bad"`
		hidden string
	}{Count: 3, Tags: []string{"a", "b"}, hidden: "x"}

	got := Headers(nil, &typed)
	assert.Equal(t, http.Header{"X-Total-Count": {"3"}, "X-Tags": {"a,b"}}, got)
	assert.Equal(t, http.Header{"A": {"1"}}, Headers(http.Header{"A": {"1"}}, 5))
	assert.Equal(t, http.Header{}, Headers(nil, (*rgb)(nil)))
}

func TestDecodeHeaders(t *testing.T) {
	t.Parallel()

	type typed struct {
		Count  *int     `json:"X-Total-Count"`
		Tags   []string `json:"X-Tags"`
		Token  string   `json:"X-Page-Token"`
		Skip   string   `json:"-"`
		hidden string   //nolint:unused // left alone by the decoder
	}
	tests := []struct {
		name    string
		header  http.Header
		dst     any
		want    any
		wantErr string
	}{
		{
			name:   "Each field from its header, a missing one left as it is",
			header: http.Header{"X-Total-Count": {"3"}, "X-Tags": {"a,b"}, "Skip": {"x"}},
			dst:    &typed{Token: "kept"},
			want:   &typed{Count: Ptr(3), Tags: []string{"a", "b"}, Token: "kept"},
		},
		{name: "A nil struct pointer is made", header: http.Header{"X-Page-Token": {"t"}}, dst: new(*typed), want: new(&typed{Token: "t"})},
		{name: "A header that does not decode", header: http.Header{"X-Total-Count": {"x"}}, dst: &typed{}, wantErr: `invalid response header X-Total-Count: invalid parameter value: "x" is no int`},
		{name: "A target that is no pointer", dst: typed{}, wantErr: "invalid parameter value: the target must be a pointer"},
		{name: "A target that is no struct", dst: new(int), wantErr: "invalid parameter value: typed headers need a struct, not int"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := DecodeHeaders(tc.header, tc.dst)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, tc.dst)
		})
	}
}
