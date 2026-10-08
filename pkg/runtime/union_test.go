// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/url"
	"strings"
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

// scalars stands in for a generated union of scalars; isNoText leaves out its string variant.
type scalars struct {
	Int  *int
	Bool *bool
	Text *string

	isNoText bool
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

// onlyA and onlyB pass their checks with one value each, as single-value enums do.
type onlyA string

func (o onlyA) Validate() error {
	if o != "a" {
		return errors.New("must be a")
	}
	return nil
}

type onlyB string

func (o onlyB) Validate() error {
	if o != "b" {
		return errors.New("must be b")
	}
	return nil
}

// left and right are objects of one shape whose checks take one side each.
type left struct {
	Side string `json:"side"`
}

func (l left) Validate() error {
	if l.Side != "left" {
		return errors.New("must be left")
	}
	return nil
}

type right struct {
	Side string `json:"side"`
}

func (r right) Validate() error {
	if r.Side != "right" {
		return errors.New("must be right")
	}
	return nil
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

// lines stands in for a generated union of a list of objects and an empty string.
type lines struct {
	List  *[]address
	Empty *string
}

func (l *lines) UnmarshalJSON(data []byte) error {
	*l = lines{}
	return UnmarshalUnion(data, l.union())
}

func (l *lines) UnmarshalForm(form *multipart.Form) error {
	*l = lines{}
	return UnmarshalUnionForm(form, nil, l.union())
}

func (l *lines) union() Union {
	return Union{IsAnyOf: true, Variants: []Variant{
		{Name: "List", Kind: KindArray, Into: Into(&l.List)},
		{Name: "Empty", Kind: KindString, Into: Into(&l.Empty)},
	}}
}

func (p *post) UnmarshalForm(form *multipart.Form) error {
	*p = post{}
	type plain post
	return UnmarshalUnionForm(form, (*plain)(p), Union{Shared: []string{"id"}, Variants: []Variant{
		{Name: "Cat", Kind: KindObject, Required: []string{"meow"}, Known: []string{"name", "meow"}, Into: Into(&p.Cat)},
		{Name: "Photo", Kind: KindObject, Required: []string{"image"}, Known: []string{"image", "caption", "tags"}, Into: Into(&p.Photo)},
	}})
}

func TestUnmarshalUnion(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, time.September, 30, 0, 0, 0, 0, time.UTC)
	hi, float, integer := "hi", 1.5, 3
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
			data: `{"name":"a","meow":true,"bark":true,"x":1}`,
			edit: func(u *Union) { u.Variants[0].IsClosed = true },
			want: holder{Dog: &dog{Name: "a", Bark: true}},
		},
		{
			name:       "Two objects with the required properties of each tie",
			data:       `{"meow":true,"bark":true}`,
			wantErr:    ErrAmbiguous,
			wantErrMsg: "more than one union variant matches: Cat and Dog",
		},
		{
			name:       "An object without the required properties of any variant",
			data:       `{"name":"a"}`,
			wantErr:    ErrNoVariant,
			wantErrMsg: "no union variant matches for a JSON object: Cat needs meow, Dog needs bark",
		},
		{
			name: "A variant with the required properties that fails to decode",
			data: `{"meow":"loud"}`,
			wantErrMsg: "no union variant matches for a JSON object: " +
				"Cat: json: cannot unmarshal string into Go struct field cat.meow of type bool",
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
		{name: "String that fails the first variant", data: `"hi"`, want: holder{Text: &hi}},
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
		{
			name: "A literal value picks by JSON value",
			data: `{"type":1.0,"name":"a"}`,
			edit: func(u *Union) {
				petsOnly(u)
				u.IsLiteral, u.Variants[0].Values = true, []string{"1"}
			},
			want: holder{Cat: &cat{Name: "a"}},
		},
		{
			name: "A string is no literal",
			data: `{"type":"1","name":"a"}`,
			edit: func(u *Union) {
				petsOnly(u)
				u.IsLiteral, u.Variants[0].Values, u.Variants[1].IsDefault = true, []string{"1"}, true
			},
			want: holder{Dog: &dog{Name: "a"}},
		},
		{
			name: "A variant takes a value without the property",
			data: `{"name":"a","bark":true}`,
			edit: func(u *Union) {
				petsOnly(u)
				u.Variants[0].IsAbsent, u.Variants[1].IsDefault = true, true
			},
			want: holder{Cat: &cat{Name: "a"}},
		},
		{name: "Any of sets every match", data: `{"name":"a"}`, edit: func(u *Union) { noRequired(u); u.IsAnyOf = true }, want: holder{Cat: &cat{Name: "a"}, Dog: &dog{Name: "a"}}},
		{name: "Any of sets every match that decodes", data: `{"meow":true,"bark":"x"}`, edit: func(u *Union) { u.IsAnyOf = true }, want: holder{Cat: &cat{Meow: true}}},
		{name: "Any of where no match decodes", data: `{"meow":1,"bark":1}`, edit: func(u *Union) { u.IsAnyOf = true }, wantErr: ErrNoVariant},
		{
			name:       "Any of needs the required properties of a variant",
			data:       `{"name":"a"}`,
			edit:       func(u *Union) { u.IsAnyOf = true },
			wantErr:    ErrNoVariant,
			wantErrMsg: "no union variant matches for a JSON object: Cat needs meow, Dog needs bark",
		},
		{name: "Any of over strings", data: `"2026-09-30T00:00:00Z"`, edit: func(u *Union) { u.IsAnyOf = true }, want: holder{When: &when, Text: new("2026-09-30T00:00:00Z")}},
		{name: "Any of without candidates", data: `true`, edit: func(u *Union) { u.IsAnyOf = true }, wantErr: ErrNoVariant},
		{
			name: "Each union in Also sets a variant too",
			data: `{"name":"a","meow":true,"bark":true}`,
			edit: func(u *Union) {
				u.Also = []Union{{Variants: u.Variants[1:2]}}
				u.Variants = u.Variants[:1]
			},
			want: holder{Cat: &cat{Name: "a", Meow: true}, Dog: &dog{Name: "a", Bark: true}},
		},
		{
			name: "A union in Also that nothing matches",
			data: `{"name":"a","meow":true}`,
			edit: func(u *Union) {
				u.Also = []Union{{Variants: u.Variants[1:2]}}
				u.Variants = u.Variants[:1]
			},
			wantErr:    ErrNoVariant,
			wantErrMsg: "no union variant matches for a JSON object: Dog needs bark",
		},
		{
			name: "A union in Also takes the shared keys",
			data: `{"id":1,"meow":true,"bark":true}`,
			edit: func(u *Union) {
				closed := u.Variants[1]
				closed.IsClosed = true
				u.Shared, u.Variants, u.Also = []string{"id", "meow"}, u.Variants[:1], []Union{{Variants: []Variant{closed}}}
			},
			want: holder{Cat: &cat{Meow: true}, Dog: &dog{Bark: true}},
		},
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
		name       string
		data       string
		variants   func(first, second *map[string]any) []Variant
		want       string
		wantErr    error
		wantErrMsg string
	}{
		{name: "A key only the first variant's shapes require", data: `{"a":1}`, want: "first"},
		{name: "Its other shape", data: `{"b":1}`, want: "first"},
		{name: "All keys of a shape of the second", data: `{"a":1,"c":2}`, want: "second"},
		{name: "A closed shape that fits", data: `{"d":1}`, want: "second"},
		{name: "What each shape needs", data: `{"x":1}`, wantErr: ErrNoVariant, wantErrMsg: "no union variant matches for a JSON object: First needs a or b, Second needs a and c"},
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
				if tc.wantErrMsg != "" {
					require.EqualError(t, err, tc.wantErrMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want == "first", first != nil)
			assert.Equal(t, tc.want == "second", second != nil)
		})
	}
}

func TestUnmarshalUnionChecks(t *testing.T) {
	t.Parallel()

	type got struct {
		A     *onlyA
		B     *onlyB
		Left  *left
		Right *right
		Any   map[string]any
	}
	letters := func(g *got) []Variant {
		return []Variant{
			{Name: "A", Kind: KindString, Into: Into(&g.A)},
			{Name: "B", Kind: KindString, Into: Into(&g.B)},
		}
	}
	sides := func(g *got) []Variant {
		return []Variant{
			{Name: "Left", Kind: KindObject, Required: []string{"side"}, Known: []string{"side"}, Into: Into(&g.Left)},
			{Name: "Right", Kind: KindObject, Required: []string{"side"}, Known: []string{"side"}, Into: Into(&g.Right)},
		}
	}
	tests := []struct {
		name       string
		data       string
		isAnyOf    bool
		variants   func(g *got) []Variant
		want       got
		wantErrMsg string
	}{
		{name: "A later variant whose checks pass", data: `"b"`, variants: letters, want: got{B: new(onlyB("b"))}},
		{name: "The first variant when its checks pass", data: `"a"`, variants: letters, want: got{A: new(onlyA("a"))}},
		{name: "The first that decodes when no checks pass", data: `"c"`, variants: letters, want: got{A: new(onlyA("c"))}},
		{name: "Any of sets only those whose checks pass", data: `"b"`, isAnyOf: true, variants: letters, want: got{B: new(onlyB("b"))}},
		{name: "Any of sets every one that decodes when no checks pass", data: `"c"`, isAnyOf: true, variants: letters, want: got{A: new(onlyA("c")), B: new(onlyB("c"))}},
		{name: "Checks break a tie of objects", data: `{"side":"right"}`, variants: sides, want: got{Right: &right{Side: "right"}}},
		{
			name:       "Objects of one rank that fail their checks are ambiguous",
			data:       `{"side":"up"}`,
			variants:   sides,
			wantErrMsg: "more than one union variant matches: Left and Right",
		},
		{
			name: "Objects of one rank that pass their checks are ambiguous",
			data: `{"side":"left"}`,
			variants: func(g *got) []Variant {
				return []Variant{sides(g)[0], {Name: "Any", Kind: KindObject, Required: []string{"side"}, Into: Into(&g.Any)}}
			},
			wantErrMsg: "more than one union variant matches: Left and Any",
		},
		{
			name: "A lower rank whose checks pass",
			data: `{"side":"up"}`,
			variants: func(g *got) []Variant {
				return []Variant{sides(g)[0], {Name: "Any", Kind: KindObject, Known: []string{}, Into: Into(&g.Any)}}
			},
			want: got{Any: map[string]any{"side": "up"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var g got
			err := UnmarshalUnion([]byte(tc.data), Union{IsAnyOf: tc.isAnyOf, Variants: tc.variants(&g)})

			if tc.wantErrMsg != "" {
				require.ErrorIs(t, err, ErrAmbiguous)
				require.EqualError(t, err, tc.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, g)
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

	_, err := set.decode([]byte(`"a"`))
	require.Error(t, err)
	assert.Equal(t, 1, *dst)
	value, err := set.decode([]byte(`2`))
	require.NoError(t, err)
	assert.Equal(t, 1, *dst)
	value.set()
	assert.Equal(t, 2, *dst)

	_, err = set.fill(&multipart.Form{Value: url.Values{"": {"a"}}})
	require.Error(t, err)
	assert.Equal(t, 2, *dst)

	var only *onlyA
	value, err = Into(&only).decode([]byte(`"b"`))
	require.NoError(t, err)
	assert.False(t, value.isValid)
}

func TestUnmarshalUnionForm(t *testing.T) {
	t.Parallel()

	r := multipartRequest(t, func(w *multipart.Writer) {
		fw, err := w.CreateFormFile("image", "a.png")
		require.NoError(t, err)
		_, err = fw.Write([]byte("PNG"))
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

func TestUnmarshalUnionFormList(t *testing.T) {
	t.Parallel()

	var got struct {
		Lines lines `json:"lines"`
	}
	require.NoError(t, DecodeForm(strings.NewReader("lines[0][city]=Rome&lines[1][city]=Oslo"), &got, true, nil))
	assert.Equal(t, lines{List: &[]address{{City: "Rome"}, {City: "Oslo"}}}, got.Lines)

	require.NoError(t, DecodeForm(strings.NewReader("lines="), &got, true, nil))
	assert.Equal(t, lines{Empty: new("")}, got.Lines)

	assert.False(t, isIndexedForm(&multipart.Form{}), "an empty form is no list")
	assert.True(t, isIndexedForm(&multipart.Form{File: map[string][]*multipart.FileHeader{"0": nil}}), "file parts count")
	assert.False(t, isIndexedForm(&multipart.Form{Value: url.Values{"0": {"a"}, "name": {"b"}}}))
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
			name: "A form text reads as a literal",
			u: func(h *holder) Union {
				u := pets(h)
				u.IsLiteral, u.Variants[0].Values, u.Variants[1].IsDefault = true, []string{"1"}, true
				return u
			},
			values: url.Values{"type": {"1"}, "name": {"Kit"}},
			want:   holder{Cat: &cat{Name: "Kit"}},
		},
		{
			name:    "A form no object variant takes",
			u:       (*holder).union,
			values:  url.Values{"meow": {"loud"}},
			wantErr: ErrNoVariant,
		},
		{
			name:    "A form that matches two variants alike",
			u:       (*holder).union,
			values:  url.Values{"meow": {"true"}, "bark": {"true"}},
			wantErr: ErrAmbiguous,
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
		Value: url.Values{"type": {"dog"}, "a[b]": {"1"}, "a": {"x"}, "empty": {}, "": {"x"}, "doc": {"text"}, "n": {"1.5"}, "on": {"true"}},
		File:  map[string][]*multipart.FileHeader{"doc": {{}}, "image": {{}}},
	}

	assert.Equal(t, map[string]json.RawMessage{
		"type":  json.RawMessage(`"dog"`),
		"n":     json.RawMessage(`1.5`),
		"on":    json.RawMessage(`true`),
		"a":     json.RawMessage(`"x"`),
		"empty": json.RawMessage(`{}`),
		"doc":   json.RawMessage(`"text"`),
		"image": json.RawMessage(`{}`),
	}, formMembers(form))
}
