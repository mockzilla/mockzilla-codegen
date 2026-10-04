// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"net/http"
	"net/url"
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
		{name: "Pipe delimited list", param: explode(StylePipeDelimited, false), value: colors, text: "color=blue|black|brown"},
		{name: "Pipe delimited object", param: explode(StylePipeDelimited, false), value: color, text: "color=R|100|G|200|B|150"},
		{name: "Deep object", param: explode(StyleDeepObject, true), value: color, text: "color[R]=100&color[G]=200&color[B]=150"},
		{name: "Deep object into a map", param: explode(StyleDeepObject, true), value: map[string]string{"R": "100"}, text: "color[R]=100"},
		{
			name:  "Deep object with a list, an object and a map inside",
			param: explode(StyleDeepObject, false),
			value: filter{Name: new("a"), Tags: []string{"x", "y"}, Size: &rgb{R: 1}, Labels: map[string]string{"k": "v"}},
			text:  "color[name]=a&color[tags]=x&color[tags]=y&color[size][R]=1&color[size][G]=0&color[size][B]=0&color[labels][k]=v",
		},
		{name: "Deep object with a list of one", param: explode(StyleDeepObject, false), value: filter{Tags: []string{"x"}}, text: "color[tags]=x"},
		{name: "Form object with its list unset", param: explode(StyleForm, false), value: filter{Name: new("a")}, text: "color=name,a"},
		{name: "JSON", param: Param{Name: "color", IsJSON: true}, value: color, text: `color={"R":100,"G":200,"B":150}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want, err := url.ParseQuery(tc.text)
			require.NoError(t, err)
			got := url.Values{}
			require.NoError(t, EncodeQuery(tc.value, tc.param, got))
			assert.Equal(t, want, got)

			dst := reflect.New(reflect.TypeOf(tc.value))
			require.NoError(t, DecodeQuery(want, tc.param, dst.Interface()))
			assert.Equal(t, tc.value, dst.Elem().Interface())
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
		{name: "Missing deep object", decode: func(dst any) error { return DecodeQuery(url.Values{"x": {"1"}}, explode(StyleDeepObject, true), dst) }, dst: new(rgb), want: rgb{}},
		{name: "Missing header", decode: func(dst any) error { return DecodeHeader(http.Header{}, required, dst) }, dst: new(string), wantErr: ErrParamMissing},
		{name: "Missing cookie", decode: func(dst any) error { return DecodeCookie(nil, required, dst) }, dst: new(string), wantErr: ErrParamMissing},
		{name: "Bad number", decode: func(dst any) error { return DecodePath("x", explode(StyleSimple, false), dst) }, dst: new(int), wantErr: ErrParamValue},
		{name: "Bad JSON", decode: func(dst any) error { return DecodePath("{", Param{IsJSON: true}, dst) }, dst: new(rgb), wantErr: ErrParamValue},
		{name: "Target that is no pointer", decode: func(dst any) error { return DecodePath("1", optional, dst) }, dst: 1, wantErr: ErrParamValue},
		{name: "Pointer target is allocated", decode: func(dst any) error { return DecodePath("blue", optional, dst) }, dst: new(*string), want: new("blue")},
		{name: "Nested deep object", decode: func(dst any) error {
			return DecodeQuery(url.Values{"color[a][b]": {"1"}, "color[a][c]": {"x", "y"}}, explode(StyleDeepObject, true), dst)
		}, dst: new(map[string]any), want: map[string]any{"a": map[string]any{"b": "1", "c": []any{"x", "y"}}}},
		{name: "Pipes inside a header", decode: func(dst any) error { return DecodeHeader(http.Header{"Color": {"a", "b"}}, optional, dst) }, dst: new([]string), want: []string{"a", "b"}},
		{name: "Cookie object exploded", decode: func(dst any) error {
			return DecodeCookie([]*http.Cookie{{Name: "R", Value: "1"}}, explode(StyleForm, true), dst)
		}, dst: new(rgb), want: rgb{R: 1}},
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
	q := url.Values{}
	require.NoError(t, EncodeQuery((*string)(nil), p, q))
	assert.Empty(t, q)

	cookies, err := EncodeCookie((*rgb)(nil), p)
	require.NoError(t, err)
	assert.Nil(t, cookies)

	_, err = EncodePath(make(chan int), p)
	require.ErrorIs(t, err, ErrParamValue)
	_, err = EncodePath([]chan int{nil}, p)
	require.ErrorIs(t, err, ErrParamValue)
	_, err = EncodeHeader(map[string]chan int{"a": nil}, p)
	require.ErrorIs(t, err, ErrParamValue)
	require.ErrorIs(t, EncodeQuery(struct{ C chan int }{}, p, q), ErrParamValue)
	require.ErrorIs(t, EncodeQuery(filter{Tags: []string{"x"}}, p, q), ErrParamValue, "only a deep object writes a list inside")
	deep := explode(StyleDeepObject, false)
	for _, value := range []any{struct{ C chan int }{}, struct{ L []chan int }{L: []chan int{nil}}, struct{ O struct{ C chan int } }{}} {
		require.ErrorIs(t, EncodeQuery(value, deep, q), ErrParamValue)
	}
	require.NoError(t, EncodeQuery(filter{Tags: []string{}, Labels: map[string]string{}}, deep, q))
	assert.Empty(t, q, "an empty list or map is left out")
	_, err = EncodePath(func() {}, Param{IsJSON: true})
	require.Error(t, err)
	require.ErrorIs(t, DecodeHeader(http.Header{}, p, 1), ErrParamValue)
	require.ErrorIs(t, DecodeCookie(nil, p, 1), ErrParamValue)
	require.ErrorIs(t, DecodeQuery(nil, p, 1), ErrParamValue)

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
