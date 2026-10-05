// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"mime/multipart"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cat struct {
	Name string `json:"name"`
	Meow bool   `json:"meow"`
}

type dog struct {
	Name string `json:"name"`
	Bark bool   `json:"bark"`
}

// holder stands in for a generated union with one field per variant.
type holder struct {
	Cat   *cat
	Dog   *dog
	Other map[string]any
	When  *time.Time
	Text  *string
	Float *float64
	Int   *int
	List  []int
}

// scalars stands in for a generated union of scalars; isNoText leaves out its string variant.
type scalars struct {
	Int  *int
	Bool *bool
	Text *string

	isNoText bool
}

type photo struct {
	Image   File     `json:"image"`
	Caption string   `json:"caption,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// post stands in for a generated union that reads forms, with a shared property.
type post struct {
	ID    string `json:"id,omitempty"`
	Cat   *cat   `json:"-"`
	Photo *photo `json:"-"`
}

func (p post) MarshalJSON() ([]byte, error) {
	type plain post
	var set []any
	if p.Cat != nil {
		set = append(set, p.Cat)
	}
	if p.Photo != nil {
		set = append(set, p.Photo)
	}
	return MarshalUnion(plain(p), set...)
}

func (p *post) UnmarshalForm(form *multipart.Form) error {
	*p = post{}
	type plain post
	return UnmarshalUnionForm(form, (*plain)(p), Union{Shared: []string{"id"}, Variants: []Variant{
		{Name: "Cat", Kind: KindObject, Required: []string{"meow"}, Known: []string{"name", "meow"}, Into: Into(&p.Cat)},
		{Name: "Photo", Kind: KindObject, Required: []string{"image"}, Known: []string{"image", "caption", "tags"}, Into: Into(&p.Photo)},
	}})
}

func (h *holder) union() Union {
	return Union{Variants: []Variant{
		{Name: "Cat", Kind: KindObject, Values: []string{"cat"}, Required: []string{"meow"}, Known: []string{"type", "name", "meow"}, Into: Into(&h.Cat)},
		{Name: "Dog", Kind: KindObject, Values: []string{"dog"}, Required: []string{"bark"}, Known: []string{"type", "name", "bark"}, Into: Into(&h.Dog)},
		{Name: "When", Kind: KindString, Into: Into(&h.When)},
		{Name: "Text", Kind: KindString, Into: Into(&h.Text)},
		{Name: "Float", Kind: KindInteger | KindNumber, Into: Into(&h.Float)},
		{Name: "Int", Kind: KindInteger, Into: Into(&h.Int)},
		{Name: "List", Kind: KindArray, Into: Into(&h.List)},
	}}
}

func (s *scalars) UnmarshalJSON(data []byte) error {
	*s = scalars{isNoText: s.isNoText}
	u := Union{Variants: []Variant{
		{Name: "Int", Kind: KindInteger, Into: Into(&s.Int)},
		{Name: "Bool", Kind: KindBool, Into: Into(&s.Bool)},
		{Name: "Text", Kind: KindString, Into: Into(&s.Text)},
	}}
	if s.isNoText {
		u.Variants = u.Variants[:2]
	}
	return UnmarshalUnion(data, u)
}

func TestUnmarshalUnion(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)
	text, float, integer := "hi", 1.5, 3
	noRequired := func(u *Union) {
		u.Variants[0].Required, u.Variants[1].Required = nil, nil
	}
	petsOnly := func(u *Union) {
		u.Discriminator = "type"
		u.Variants = u.Variants[:2]
	}

	tests := []struct {
		name       string
		data       string
		edit       func(u *Union)
		want       holder
		wantErr    error
		wantErrMsg string
	}{
		{name: "Object with the required properties", data: `{"name":"a","meow":true}`, want: holder{Cat: &cat{Name: "a", Meow: true}}},
		{name: "Unknown keys lower the score", data: `{"name":"a","bark":true}`, edit: noRequired, want: holder{Dog: &dog{Name: "a", Bark: true}}},
		{
			name:       "Two perfect matches are ambiguous",
			data:       `{"name":"a"}`,
			edit:       noRequired,
			wantErr:    ErrAmbiguous,
			wantErrMsg: "more than one union variant matches: Cat and Dog",
		},
		{
			name: "A closed variant is ruled out by an unknown key",
			data: `{"name":"a","meow":true,"x":1}`,
			edit: func(u *Union) { u.Variants[0].IsClosed = true },
			want: holder{Dog: &dog{Name: "a"}},
		},
		{
			name: "Shared keys are not unknown",
			data: `{"id":1,"name":"a","meow":true}`,
			edit: func(u *Union) {
				noRequired(u)
				u.Shared = []string{"id"}
			},
			want: holder{Cat: &cat{Name: "a", Meow: true}},
		},
		{name: "String that decodes into the first string variant", data: `"2026-09-30T00:00:00Z"`, want: holder{When: &when}},
		{name: "String that fails the first variant", data: `"hi"`, want: holder{Text: &text}},
		{name: "Integer prefers the integer variant", data: `3`, want: holder{Int: &integer}},
		{name: "Fraction takes the float variant", data: ` 1.5 `, want: holder{Float: &float}},
		{name: "Null sets nothing", data: `null`},
		{name: "No variant takes the kind", data: `true`, wantErr: ErrNoVariant, wantErrMsg: "no union variant matches for a JSON boolean"},
		{name: "Every candidate fails to decode", data: `["a"]`, wantErr: ErrNoVariant},
		{name: "No JSON value", data: `x`, wantErr: ErrNoVariant, wantErrMsg: `no union variant matches: "x" is no JSON value`},
		{name: "Broken object", data: `{"a"`, wantErrMsg: "unexpected end of JSON input"},
		{name: "Discriminator picks its variant", data: `{"type":"dog","name":"a"}`, edit: petsOnly, want: holder{Dog: &dog{Name: "a"}}},
		{
			name:       "Unknown discriminator value",
			data:       `{"type":"fish"}`,
			edit:       petsOnly,
			wantErr:    ErrUnknownDiscriminator,
			wantErrMsg: `unknown discriminator value "fish", want one of cat, dog`,
		},
		{name: "Missing discriminator falls back to the shape", data: `{"name":"a","meow":true}`, edit: petsOnly, want: holder{Cat: &cat{Name: "a", Meow: true}}},
		{
			name: "Default variant takes an unknown value",
			data: `{"type":"fish","name":"a"}`,
			edit: func(u *Union) {
				petsOnly(u)
				u.Variants[1].IsDefault = true
			},
			want: holder{Dog: &dog{Name: "a"}},
		},
		{
			name: "Discriminator value that is no string",
			data: `{"type":1,"name":"a"}`,
			edit: func(u *Union) {
				petsOnly(u)
				u.Variants[0].Values = []string{"1"}
			},
			want: holder{Cat: &cat{Name: "a"}},
		},
		{name: "Any of sets every match", data: `{"name":"a"}`, edit: func(u *Union) { noRequired(u); u.IsAnyOf = true }, want: holder{Cat: &cat{Name: "a"}, Dog: &dog{Name: "a"}}},
		{name: "Any of falls back to the first that decodes", data: `{"name":"a"}`, edit: func(u *Union) { u.IsAnyOf = true }, want: holder{Cat: &cat{Name: "a"}}},
		{name: "Any of over strings", data: `"2026-09-30T00:00:00Z"`, edit: func(u *Union) { u.IsAnyOf = true }, want: holder{When: &when, Text: new("2026-09-30T00:00:00Z")}},
		{name: "Any of without candidates", data: `true`, edit: func(u *Union) { u.IsAnyOf = true }, wantErr: ErrNoVariant},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var h holder
			u := h.union()
			if tc.edit != nil {
				tc.edit(&u)
			}

			err := UnmarshalUnion([]byte(tc.data), u)

			if tc.wantErrMsg != "" || tc.wantErr != nil {
				require.Error(t, err)
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
				}
				if tc.wantErrMsg != "" {
					require.EqualError(t, err, tc.wantErrMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, h)
		})
	}
}

func TestUnmarshalUnionOpenVariant(t *testing.T) {
	t.Parallel()

	var h holder
	u := Union{Discriminator: "type", Variants: []Variant{
		{Name: "Cat", Kind: KindObject, Values: []string{"cat"}, Into: Into(&h.Cat)},
		{Name: "Other", Kind: KindObject, Into: Into(&h.Other)},
	}}

	require.NoError(t, UnmarshalUnion([]byte(`{"type":"fish"}`), u))
	assert.Equal(t, holder{Other: map[string]any{"type": "fish"}}, h)
}

func TestUnmarshalUnionShapes(t *testing.T) {
	t.Parallel()

	shapes := func(required ...string) Shape {
		return Shape{Required: required, Known: required}
	}
	tests := []struct {
		name     string
		data     string
		variants func(first, second *map[string]any) []Variant
		want     string
		wantErr  error
	}{
		{name: "A key only the first variant's shapes require", data: `{"a":1}`, want: "first"},
		{name: "Its other shape", data: `{"b":1}`, want: "first"},
		{name: "All keys of a shape of the second", data: `{"a":1,"c":2}`, want: "second"},
		{name: "A closed shape that fits", data: `{"d":1}`, want: "second"},
		{
			name: "A variant whose shapes are all closed to a key is left out",
			data: `{"q":1}`,
			variants: func(first, _ *map[string]any) []Variant {
				return []Variant{{Name: "First", Kind: KindObject, Shapes: []Shape{{Known: []string{"z"}, IsClosed: true}}, Into: Into(first)}}
			},
			wantErr: ErrNoVariant,
		},
		{
			name: "The perfect shape counts on a tie",
			data: `{"a":1,"b":2}`,
			variants: func(first, second *map[string]any) []Variant {
				return []Variant{
					{Name: "First", Kind: KindObject, Shapes: []Shape{shapes("a"), {Known: []string{"a", "b"}}}, Into: Into(first)},
					{Name: "Second", Kind: KindObject, Into: Into(second)},
				}
			},
			wantErr: ErrAmbiguous,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var first, second map[string]any
			variants := []Variant{
				{Name: "First", Kind: KindObject, Shapes: []Shape{shapes("a"), shapes("b")}, Into: Into(&first)},
				{Name: "Second", Kind: KindObject, Shapes: []Shape{shapes("a", "c"), {Required: []string{"d"}, Known: []string{"d"}, IsClosed: true}}, Into: Into(&second)},
			}
			if tc.variants != nil {
				variants = tc.variants(&first, &second)
			}

			err := UnmarshalUnion([]byte(tc.data), Union{Variants: variants})

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want == "first", first != nil)
			assert.Equal(t, tc.want == "second", second != nil)
		})
	}
}

func TestUnmarshalUnionText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		text       string
		isNoText   bool
		want       scalars
		wantErrMsg string
	}{
		{name: "An integer goes to the integer variant", text: "30", want: scalars{Int: new(30)}},
		{name: "A boolean goes to the boolean variant", text: "true", want: scalars{Bool: new(true)}},
		{name: "Other text is a string", text: "abc", want: scalars{Text: new("abc")}},
		{name: "A number no variant takes is a string", text: "1.5", want: scalars{Text: new("1.5")}},
		{name: "Text that only starts like a boolean is a string", text: "tomato", want: scalars{Text: new("tomato")}},
		{name: "Space around a number keeps it a string", text: " 30", want: scalars{Text: new(" 30")}},
		{name: "Quotes stay in the string", text: `"a"`, want: scalars{Text: new(`"a"`)}},
		{name: "Empty text is an empty string", text: "", want: scalars{Text: new("")}},
		{
			name: "Without a string variant other text is an error", text: "abc", isNoText: true,
			wantErrMsg: "no union variant matches for a JSON string",
		},
		{
			name: "Without a string variant a number keeps its own error", text: "1.5", isNoText: true,
			wantErrMsg: "no union variant matches for a JSON number",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := scalars{isNoText: tc.isNoText}
			err := UnmarshalUnionText([]byte(tc.text), s.UnmarshalJSON)

			if tc.wantErrMsg != "" {
				require.ErrorIs(t, err, ErrNoVariant)
				require.EqualError(t, err, tc.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, s)
		})
	}
}

func TestInto(t *testing.T) {
	t.Parallel()

	n := 1
	dst := &n
	set := Into(&dst)

	require.Error(t, set.decode([]byte(`"a"`)))
	assert.Equal(t, 1, *dst)
	require.NoError(t, set.decode([]byte(`2`)))
	assert.Equal(t, 2, *dst)

	require.Error(t, set.fill(&multipart.Form{Value: url.Values{"": {"a"}}}))
	assert.Equal(t, 2, *dst)
}

func TestUnmarshalUnionForm(t *testing.T) {
	t.Parallel()

	r := multipartRequest(t, func(w *multipart.Writer) {
		part, err := w.CreateFormFile("image", "a.png")
		require.NoError(t, err)
		_, err = part.Write([]byte("PNG"))
		require.NoError(t, err)
		require.NoError(t, w.WriteField("caption", "sun"))
		require.NoError(t, w.WriteField("tags", "a"))
		require.NoError(t, w.WriteField("tags", "b"))
	})
	require.NoError(t, r.ParseMultipartForm(1<<20))

	var got post
	require.NoError(t, got.UnmarshalForm(r.MultipartForm))
	require.NotNil(t, got.Photo)
	assert.Nil(t, got.Cat)
	assert.Equal(t, "sun", got.Photo.Caption)
	assert.Equal(t, []string{"a", "b"}, got.Photo.Tags)
	data, err := got.Photo.Image.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "PNG", string(data))

	require.NoError(t, got.UnmarshalForm(&multipart.Form{Value: url.Values{"id": {"7"}, "name": {"Tom"}, "meow": {"true"}}}))
	assert.Equal(t, post{ID: "7", Cat: &cat{Name: "Tom", Meow: true}}, got)
}

func TestUnmarshalUnionFormPicks(t *testing.T) {
	t.Parallel()

	pets := func(h *holder) Union {
		return Union{Discriminator: "type", Variants: h.union().Variants[:2]}
	}
	tests := []struct {
		name    string
		u       func(h *holder) Union
		values  url.Values
		want    holder
		wantErr error
	}{
		{
			name:   "The discriminator picks",
			u:      pets,
			values: url.Values{"type": {"dog"}, "name": {"Rex"}, "bark": {"true"}},
			want:   holder{Dog: &dog{Name: "Rex", Bark: true}},
		},
		{
			name:    "A value no variant takes",
			u:       pets,
			values:  url.Values{"type": {"fish"}},
			wantErr: ErrUnknownDiscriminator,
		},
		{
			name:   "Required keys pick, nested keys count by their first name",
			u:      (*holder).union,
			values: url.Values{"bark": {"true"}, "extra[x]": {"a"}},
			want:   holder{Dog: &dog{Bark: true}},
		},
		{
			name:   "anyOf sets every match",
			u:      func(h *holder) Union { u := h.union(); u.IsAnyOf = true; return u },
			values: url.Values{"name": {"Kit"}, "meow": {"true"}, "bark": {"false"}},
			want:   holder{Cat: &cat{Name: "Kit", Meow: true}, Dog: &dog{Name: "Kit"}},
		},
		{
			name:    "A form no object variant takes",
			u:       (*holder).union,
			values:  url.Values{"meow": {"loud"}, "bark": {"loud"}},
			wantErr: ErrNoVariant,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got holder
			err := UnmarshalUnionForm(&multipart.Form{Value: tc.values}, nil, tc.u(&got))

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUnmarshalUnionFormShared(t *testing.T) {
	t.Parallel()

	var got holder
	err := UnmarshalUnionForm(&multipart.Form{}, holder{}, got.union())

	require.ErrorIs(t, err, ErrParamValue)
}

func TestFormMembers(t *testing.T) {
	t.Parallel()

	form := &multipart.Form{
		Value: url.Values{"type": {"dog"}, "a[b]": {"1"}, "a": {"x"}, "empty": {}, "": {"x"}, "doc": {"text"}},
		File:  map[string][]*multipart.FileHeader{"doc": {{}}, "image": {{}}},
	}

	assert.Equal(t, map[string]json.RawMessage{
		"type":  json.RawMessage(`"dog"`),
		"a":     json.RawMessage(`"x"`),
		"empty": json.RawMessage(`{}`),
		"doc":   json.RawMessage(`"text"`),
		"image": json.RawMessage(`{}`),
	}, formMembers(form))
}
