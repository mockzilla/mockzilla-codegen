// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testObjects = []Object{
	{Name: "Meta", Extra: &Prop{IsNullable: true}},
	{Name: "Owner", Props: []Prop{
		{Key: "city", Default: `"Berlin"`},
		{Key: "id", IsRequired: true},
	}},
	{Name: "Pet", IsClosed: true, Props: []Prop{
		{Key: "age", Default: `1`},
		{Key: "bad", Default: `{`},
		{Key: "byName", Values: &Prop{Object: "Owner"}},
		{Key: "meta", Object: "Meta", Default: `{"k":"v"}`},
		{Key: "name", IsRequired: true},
		{Key: "none", Default: `null`},
		{Key: "owner", Object: "Owner"},
		{Key: "strict", Object: "Strict"},
		{Key: "tag", IsNullable: true},
		{Key: "tags", Items: &Prop{}, Default: `["a",2,true]`},
		{Key: "toys", Items: &Prop{Object: "Owner"}, Default: `[{"id":1}]`},
	}},
	{Name: "Strict", Extra: &Prop{}},
	{Name: "Upload", Props: []Prop{
		{Key: "note", Default: `"none"`},
		{Key: "photo", IsRequired: true},
	}},
}

func TestPresenceJSON(t *testing.T) {
	t.Parallel()

	pet := Prop{Object: "Pet"}
	defaults := `"age":1,"meta":{"k":"v"},"none":null,"tags":["a",2,true],"toys":[{"id":1}]`
	tests := []struct {
		name      string
		isChecked bool
		p         Prop
		body      string
		isSame    bool
		want      string
		wantErr   string
	}{
		{name: "Missing properties get their defaults", isChecked: true, p: pet, body: `{"name":"Rex"}`, want: `{"name":"Rex",` + defaults + `}`},
		{name: "Nested objects get their defaults", isChecked: true, p: pet, body: `{"name":"Rex","age":2,"meta":{},"none":1,"tags":[],"toys":[],"owner":{"id":1},"byName":{"x":{"id":2}}}`, want: `{"name":"Rex","age":2,"meta":{},"none":1,"tags":[],"toys":[],"owner":{"city":"Berlin","id":1},"byName":{"x":{"city":"Berlin","id":2}}}`},
		{name: "A body that needs no default comes back as it was", isChecked: true, p: pet, body: ` {"name":"Rex","age":2,"meta":{},"none":1,"tags":[],"toys":[]} `, isSame: true},
		{name: "A null is an error where a default exists", isChecked: true, p: Prop{Object: "Owner"}, body: `{"id":1,"city":null}`, wantErr: "body.city: must not be null"},
		{name: "A missing required key is an error", isChecked: true, p: pet, body: `{"owner":{}}`, wantErr: "body.name: is required; body.owner.id: is required"},
		{name: "A null property is an error", isChecked: true, p: pet, body: `{"name":null,"tag":null}`, wantErr: "body.name: must not be null"},
		{name: "An unknown key of a closed object is an error", isChecked: true, p: pet, body: `{"name":"Rex","x":1}`, wantErr: "body.x: is not allowed"},
		{name: "A null list item is an error", isChecked: true, p: pet, body: `{"name":"Rex","tags":["a",null]}`, wantErr: "body.tags[1]: must not be null"},
		{name: "A null map value is an error", isChecked: true, p: pet, body: `{"name":"Rex","byName":{"x":null}}`, wantErr: `body.byName["x"]: must not be null`},
		{name: "Other keys are checked by Extra", isChecked: true, p: pet, body: `{"name":"Rex","meta":{"a":null},"strict":{"b":null}}`, wantErr: `body.strict["b"]: must not be null`},
		{name: "A null body is an error", isChecked: true, p: pet, body: `null`, wantErr: "body: must not be null"},
		{name: "A nullable body takes null", isChecked: true, p: Prop{IsNullable: true}, body: `null`, want: `null`},
		{name: "A value of another kind is left to the decoder", isChecked: true, p: pet, body: `[1]`, isSame: true},
		{name: "Without checks only defaults are set", p: pet, body: `{"x":null,"name":null,"owner":{}}`, want: `{"x":null,"name":null,"owner":{"city":"Berlin"},` + defaults + `}`},
		{name: "An empty body is left to the decoder", isChecked: true, p: pet, body: "  ", isSame: true},
		{name: "A body that is no JSON is left to the decoder", isChecked: true, p: pet, body: `{"name":`, isSame: true},
		{name: "Data after the value is left to the decoder", isChecked: true, p: pet, body: `{} {}`, isSame: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := Presence{IsChecked: tc.isChecked, Objects: testObjects}
			got, err := b.JSON(strings.NewReader(tc.body), tc.p)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				assert.True(t, IsValidation(err))
				return
			}
			require.NoError(t, err)
			data, err := io.ReadAll(got)
			require.NoError(t, err)
			if tc.isSame {
				assert.Equal(t, tc.body, string(data))
				return
			}
			assert.JSONEq(t, tc.want, string(data))
		})
	}
}

func TestPresenceJSONRules(t *testing.T) {
	t.Parallel()

	b := Presence{IsChecked: true, Objects: testObjects}
	_, err := b.JSON(strings.NewReader(`{"tag":null,"tags":[null],"x":1}`), Prop{Object: "Pet"})

	assert.Equal(t, ValidationErrors{
		{Field: "body.name", Message: "is required", Rule: RuleRequired},
		{Field: "body.tags[0]", Message: "must not be null", Rule: RuleType},
		{Field: "body.x", Message: "is not allowed", Rule: RuleAdditionalProperties, Limit: false},
	}, err)
}

func TestPresenceForm(t *testing.T) {
	t.Parallel()

	pet := Prop{Object: "Pet"}
	defaults := url.Values{"age": {"1"}, "meta": {`{"k":"v"}`}, "tags": {"a", "2", "true"}, "toys": {`[{"id":1}]`}}
	with := func(v url.Values) url.Values {
		out := url.Values{}
		for _, m := range []url.Values{defaults, v} {
			for key, values := range m {
				out[key] = values
			}
		}
		return out
	}
	declared := Encoding{"name": "application/json", "tag": "application/json", "tags": "application/json", "toys": "application/json"}
	tests := []struct {
		name    string
		p       Prop
		enc     Encoding
		body    string
		want    url.Values
		wantErr string
	}{
		{name: "Missing fields get their defaults", p: pet, body: "name=Rex", want: with(url.Values{"name": {"Rex"}})},
		{name: "A field declared JSON gets its default as JSON", p: pet, enc: declared, body: `name="Rex"&toys={"id":3}`, want: with(url.Values{"name": {`"Rex"`}, "tags": {`["a",2,true]`}, "toys": {`{"city":"Berlin","id":3}`}})},
		{name: "A field declared JSON takes null when nullable", p: pet, enc: declared, body: `name="Rex"&tag=null&tags=[]&toys=[]`, want: with(url.Values{"name": {`"Rex"`}, "tag": {"null"}, "tags": {"[]"}, "toys": {"[]"}})},
		{name: "A null in a field declared JSON is an error", p: pet, enc: declared, body: "name=null", wantErr: "body.name: must not be null"},
		{name: "A field declared JSON that is no JSON is left to the decoder", p: pet, enc: declared, body: "name=Rex&tags=[]&toys=[]", want: with(url.Values{"name": {"Rex"}, "tags": {"[]"}, "toys": {"[]"}})},
		{name: "Items of a list declared JSON are checked one by one", p: pet, enc: declared, body: `name="Rex"&tags=[]&toys={"id":3}&toys={}`, wantErr: "body.toys[1].id: is required"},
		{name: "Items of a list in parts of their own get their defaults", p: pet, body: `name=Rex&toys={"id":3}&toys={"id":4}`, want: with(url.Values{"name": {"Rex"}, "toys": {`{"city":"Berlin","id":3}`, `{"city":"Berlin","id":4}`}})},
		{name: "Items under an empty bracket are checked but not filled", p: pet, body: `name=Rex&toys[]={"id":3}&toys[]={"id":4}`, want: url.Values{
			"age": {"1"}, "meta": {`{"k":"v"}`}, "tags": {"a", "2", "true"}, "name": {"Rex"}, "toys[]": {`{"id":3}`, `{"id":4}`},
		}},
		{name: "An item under an empty bracket is checked", p: pet, body: `name=Rex&toys[]={}`, wantErr: "body.toys[0].id: is required"},
		{name: "Nested fields get their defaults", p: pet, body: "name=Rex&owner[id]=1&byName[x][id]=2&toys[0][id]=3", want: url.Values{
			"age": {"1"}, "meta": {`{"k":"v"}`}, "tags": {"a", "2", "true"}, "name": {"Rex"}, "owner[id]": {"1"}, "owner[city]": {"Berlin"},
			"byName[x][id]": {"2"}, "byName[x][city]": {"Berlin"}, "toys[0][id]": {"3"}, "toys[0][city]": {"Berlin"},
		}},
		{name: "A field of JSON gets the defaults inside it", p: pet, body: `name=Rex&owner={"id":1}`, want: with(url.Values{"name": {"Rex"}, "owner": {`{"city":"Berlin","id":1}`}})},
		{name: "A field of JSON that needs no default stays", p: pet, body: `name=Rex&owner={"id":1,"city":"Bonn"}`, want: with(url.Values{"name": {"Rex"}, "owner": {`{"id":1,"city":"Bonn"}`}})},
		{name: "A field of text for an object is left to the decoder", p: pet, body: "name=Rex&owner=abc&strict={x", want: with(url.Values{"name": {"Rex"}, "owner": {"abc"}, "strict": {"{x"}})},
		{name: "A missing required field is an error", p: pet, body: "age=2", wantErr: "body.name: is required"},
		{name: "An unknown field of a closed object is an error", p: pet, body: "name=Rex&x=1", wantErr: "body.x: is not allowed"},
		{name: "A null in a field of JSON is an error", p: pet, body: `name=Rex&owner={"id":null}`, wantErr: "body.owner.id: must not be null"},
		{name: "Other keys are checked by Extra", p: pet, body: `name=Rex&strict[a]={"b":1}`, want: with(url.Values{"name": {"Rex"}, "strict[a]": {`{"b":1}`}})},
		{name: "A form that is no object is not checked", p: Prop{}, body: "x=1", want: url.Values{"x": {"1"}}},
		{name: "An empty body is left to the decoder", p: pet, body: "", want: url.Values{}},
		{name: "A body that does not parse is left to the decoder", p: pet, body: "a=%zz", want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := Presence{IsChecked: true, Objects: testObjects}
			got, err := b.Form(strings.NewReader(tc.body), tc.p, tc.enc)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			data, err := io.ReadAll(got)
			require.NoError(t, err)
			values, err := url.ParseQuery(string(data))
			if tc.want == nil {
				assert.Equal(t, tc.body, string(data))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, values)
		})
	}
}

func TestPresenceMultipart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fields  map[string]string
		file    string
		want    url.Values
		wantErr string
	}{
		{name: "A file part counts as its field", file: "photo", want: url.Values{"note": {"none"}}},
		{name: "A text field keeps its value", fields: map[string]string{"note": "hi", "photo": "x"}, want: url.Values{"note": {"hi"}, "photo": {"x"}}},
		{name: "A missing file part is an error", fields: map[string]string{"note": "hi"}, wantErr: "body.photo: is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			for _, name := range SortedKeys(tc.fields) {
				require.NoError(t, mw.WriteField(name, tc.fields[name]))
			}
			if tc.file != "" {
				fw, err := mw.CreateFormFile(tc.file, "a.png")
				require.NoError(t, err)
				_, err = fw.Write([]byte("png"))
				require.NoError(t, err)
			}
			require.NoError(t, mw.Close())
			r := httptest.NewRequest(http.MethodPost, "/", &buf)
			r.Header.Set("Content-Type", mw.FormDataContentType())

			err := Presence{IsChecked: true, Objects: testObjects}.Multipart(r, Prop{Object: "Upload"}, 0, nil)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, url.Values(r.MultipartForm.Value))
		})
	}
}

func TestPresenceReadErrors(t *testing.T) {
	t.Parallel()

	b := Presence{Objects: testObjects}
	broken := errors.New("broken")
	_, err := b.JSON(iotest.ErrReader(broken), Prop{})
	require.ErrorIs(t, err, broken)
	_, err = b.Form(iotest.ErrReader(broken), Prop{}, nil)
	require.ErrorIs(t, err, broken)

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("x"))
	r.Header.Set("Content-Type", "text/plain")
	err = b.Multipart(r, Prop{}, 0, nil)
	require.Error(t, err)
	assert.False(t, IsValidation(err))
}
