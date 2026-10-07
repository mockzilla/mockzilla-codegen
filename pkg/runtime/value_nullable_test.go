// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// node holds a Nullable of itself, which compiles because a Nullable keeps a pointer.
type node struct {
	Name   string         `json:"name"`
	Parent Nullable[node] `json:"parent,omitzero"`
}

// patch is a body with a Nullable of each kind a form or a query carries.
type patch struct {
	Name    Nullable[string]  `json:"name,omitzero"`
	Count   Nullable[int]     `json:"count,omitzero"`
	Address Nullable[address] `json:"address,omitzero"`
	File    Nullable[File]    `json:"file,omitzero"`
	Scores  []Nullable[int]   `json:"scores,omitzero"`
}

func TestNullableStates(t *testing.T) {
	t.Parallel()

	type state struct {
		Value    int
		HasValue bool
		IsSet    bool
		IsNull   bool
		IsZero   bool
		Or       int
	}
	tests := []struct {
		name string
		n    Nullable[int]
		want state
	}{
		{name: "The zero value is absent", want: state{IsZero: true, Or: 7}},
		{name: "Null is set without a value", n: Null[int](), want: state{IsSet: true, IsNull: true, Or: 7}},
		{name: "A value is set", n: Some(3), want: state{Value: 3, HasValue: true, IsSet: true, Or: 3}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, ok := tc.n.Get()
			got := state{Value: v, HasValue: ok, IsSet: tc.n.IsSet(), IsNull: tc.n.IsNull(), IsZero: tc.n.IsZero(), Or: tc.n.Or(7)}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNullableJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(node{Name: "a", Parent: Some(node{Name: "b", Parent: Null[node]()})})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"a","parent":{"name":"b","parent":null}}`, string(data))

	var n node
	require.NoError(t, json.Unmarshal([]byte(`{"name":"a","parent":{"name":"b","parent":null}}`), &n))
	parent, ok := n.Parent.Get()
	require.True(t, ok)
	assert.Equal(t, "b", parent.Name)
	assert.True(t, parent.Parent.IsNull())

	var orphan node
	require.NoError(t, json.Unmarshal([]byte(`{"name":"c"}`), &orphan))
	assert.False(t, orphan.Parent.IsSet())

	var number Nullable[int]
	require.Error(t, json.Unmarshal([]byte(`"x"`), &number))
}

func TestNullableReflection(t *testing.T) {
	t.Parallel()

	assert.Equal(t, reflect.TypeFor[int](), valueType(reflect.TypeFor[*Nullable[int]]()))
	assert.Equal(t, reflect.TypeFor[string](), valueType(reflect.TypeFor[string]()))

	_, ok := held(reflect.Value{})
	assert.False(t, ok)
	_, ok = held(reflect.ValueOf(Nullable[int]{}))
	assert.False(t, ok)
	_, ok = held(reflect.ValueOf((*int)(nil)))
	assert.False(t, ok)
	v, ok := held(reflect.ValueOf(new(Some(4))))
	require.True(t, ok)
	assert.Equal(t, 4, v.Interface())

	_, ok = targetOf(reflect.ValueOf(Nullable[int]{}))
	assert.False(t, ok, "a value that is not addressable")
	_, ok = targetOf(reflect.ValueOf(new(3)).Elem())
	assert.False(t, ok, "no Nullable")
	n := Some(1)
	target, ok := targetOf(reflect.ValueOf(&n).Elem())
	require.True(t, ok)
	target.SetInt(2)
	assert.Equal(t, Some(2), n)
}

func TestNullableParams(t *testing.T) {
	t.Parallel()

	type query struct {
		Limit  Nullable[int]     `json:"limit,omitzero"`
		Filter Nullable[address] `json:"filter,omitzero"`
	}
	in := query{Limit: Some(5), Filter: Some(address{City: "Rome"})}
	q := url.Values{}
	require.NoError(t, EncodeQuery(in.Limit, Param{Name: "limit", Style: StyleForm, IsExplode: true}, q))
	require.NoError(t, EncodeQuery(in.Filter, Param{Name: "filter", Style: StyleDeepObject, IsExplode: true}, q))
	require.NoError(t, EncodeQuery(Null[int](), Param{Name: "skip", Style: StyleForm, IsExplode: true}, q))
	assert.Equal(t, url.Values{"limit": {"5"}, "filter[city]": {"Rome"}, "filter[country]": {""}}, q)

	var out query
	require.NoError(t, DecodeQuery(q, Param{Name: "limit", Style: StyleForm, IsExplode: true}, &out.Limit))
	require.NoError(t, DecodeQuery(q, Param{Name: "filter", Style: StyleDeepObject, IsExplode: true}, &out.Filter))
	assert.Equal(t, in, out)

	h := Headers(nil, struct {
		Rate Nullable[int] `json:"X-Rate"`
		Gone Nullable[int] `json:"X-Gone"`
	}{Rate: Some(7), Gone: Null[int]()})
	assert.Equal(t, http.Header{"X-Rate": {"7"}}, h)
}

func TestNullableForm(t *testing.T) {
	t.Parallel()

	in := patch{Name: Some("a"), Count: Null[int](), Address: Some(address{City: "Rome"}), Scores: []Nullable[int]{Some(1), Null[int]()}}
	form, err := EncodeForm(in, nil)
	require.NoError(t, err)
	assert.Equal(t, url.Values{"name": {"a"}, "address[city]": {"Rome"}, "address[country]": {""}, "scores": {"1"}}, form)

	var out patch
	require.NoError(t, DecodeForm(bytes.NewReader([]byte(form.Encode())), &out, true, nil))
	assert.Equal(t, Some("a"), out.Name)
	assert.False(t, out.Count.IsSet())
	assert.Equal(t, Some(address{City: "Rome"}), out.Address)
	assert.Equal(t, []Nullable[int]{Some(1)}, out.Scores)
}

func TestNullableMultipart(t *testing.T) {
	t.Parallel()

	in := patch{Name: Some("a"), Address: Some(address{City: "Rome"}), File: Some(NewFile([]byte("hi"), "a.txt", "text/plain"))}
	data, contentType := multipartOf(t, &in)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
	req.Header.Set("Content-Type", contentType)

	var out patch
	require.NoError(t, DecodeMultipart(req, &out, 0, nil))
	assert.Equal(t, Some("a"), out.Name)
	assert.False(t, out.Count.IsSet())
	assert.Equal(t, Some(address{City: "Rome"}), out.Address)
	f, ok := out.File.Get()
	require.True(t, ok)
	content, err := f.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "hi", string(content))

	noParts := multipartRequest(t, func(*multipart.Writer) {})
	require.NoError(t, DecodeMultipart(noParts, &out, 0, nil))
}
