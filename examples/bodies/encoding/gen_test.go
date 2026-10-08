// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package encoding

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// service answers with what came in; GetParcel puts the Accept it got in the note.
type service struct{}

func (service) SendParcel(_ context.Context, opts *SendParcelServiceRequestOptions) (*SendParcelResponseData, error) {
	p := opts.Body
	out := &Receipt{ID: &p.ID, To: p.To, Stops: p.Stops, Note: p.Note}
	if p.Doc != nil {
		out.DocType = new(p.Doc.ContentType())
	}
	if p.Photo != nil {
		out.PhotoType = new(p.Photo.ContentType())
	}
	return NewSendParcelResponseData(out), nil
}

func (service) GetParcel(_ context.Context, opts *GetParcelServiceRequestOptions) (*GetParcelResponseData, error) {
	return NewGetParcelResponseData200(&Receipt{ID: &opts.PathParams.ID, Note: new(opts.RawRequest.Header.Get("Accept"))}), nil
}

func (service) PrintLabel(_ context.Context, opts *PrintLabelServiceRequestOptions) (*PrintLabelResponseData, error) {
	return NewPrintLabelResponseData(opts.Body), nil
}

func (service) CreateCustomer(_ context.Context, opts *CreateCustomerServiceRequestOptions) (*CreateCustomerResponseData, error) {
	return NewCreateCustomerResponseData(opts.Body), nil
}

func TestSendParcel(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	got, err := c.SendParcel(t.Context(), &SendParcelRequestOptions{Body: &Parcel{
		ID:    "p1",
		Doc:   new(runtime.NewFile([]byte("%PDF"), "a.pdf", "")),
		Photo: new(runtime.NewFile([]byte("JPG"), "a.jpg", "image/jpeg")),
		To:    &Address{City: "Rome"},
		Stops: []Address{{City: "Pisa"}, {City: "Siena", Zip: new("53100")}},
		Note:  new("fragile"),
	}})

	require.NoError(t, err)
	assert.Equal(t, &Receipt{
		ID:        new("p1"),
		DocType:   new("application/pdf"),
		PhotoType: new("image/jpeg"),
		To:        &Address{City: "Rome", Zip: new("00000")},
		Stops:     []Address{{City: "Pisa", Zip: new("00000")}, {City: "Siena", Zip: new("53100")}},
		Note:      new("fragile"),
	}, got)
}

func TestSendParcelStopsInBrackets(t *testing.T) {
	t.Parallel()

	c, err := NewClient("http://localhost:1")
	require.NoError(t, err)
	req, err := c.SendParcelRequest(t.Context(), &SendParcelRequestOptions{Body: &Parcel{ID: "p1", Stops: []Address{{City: "Pisa"}}}})
	require.NoError(t, err)
	require.NoError(t, req.ParseMultipartForm(1<<20))

	assert.Equal(t, []string{"Pisa"}, req.MultipartForm.Value["stops[0][city]"])
}

func TestCreateCustomer(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	var body Customer
	require.NoError(t, json.Unmarshal([]byte(`{
		"address": {"city": "Rome"}, "expand": ["a", "b"], "items": [{"price": "p1", "quantity": 2}],
		"returnUrl": "https://x.test/a b", "tags": ["x", "y,z"]
	}`), &body))
	opts := &CreateCustomerRequestOptions{Body: &body}

	req, err := c.CreateCustomerRequest(t.Context(), opts)
	require.NoError(t, err)
	sent, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, "address%5Bcity%5D=Rome&expand%5B0%5D=a&expand%5B1%5D=b&items%5B0%5D%5Bprice%5D=p1&items%5B0%5D%5Bquantity%5D=2"+
		"&returnUrl=https%3A%2F%2Fx.test%2Fa%20b&tags=x,y%2Cz", string(sent))

	got, err := c.CreateCustomer(t.Context(), opts)
	require.NoError(t, err)
	data, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"address": {"city": "Rome"}, "expand": ["a", "b"], "items": [{"price": "p1", "quantity": 2}],
		"returnUrl": "https://x.test/a b", "tags": ["x", "y,z"]
	}`, string(data))
}

func TestCreateCustomerFromOtherClients(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "Brackets as most form clients write them",
			body: "address[city]=Rome&expand[0]=a&items[0][price]=p1&items[0][quantity]=2&returnUrl=https://x.test",
			want: `{"address": {"city": "Rome"}, "expand": ["a"], "items": [{"price": "p1", "quantity": 2}], "returnUrl": "https://x.test"}`,
		},
		{name: "Empty brackets for a list", body: "expand[]=a&expand[]=b", want: `{"expand": ["a", "b"]}`},
		{name: "A list under its bare name", body: "expand=a&expand=b", want: `{"expand": ["a", "b"]}`},
		{name: "An empty value clears a field", body: "address=&returnUrl=", want: `{"address": "", "returnUrl": ""}`},
	}

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := http.Post(srv.URL+"/customers", "application/x-www-form-urlencoded", strings.NewReader(tc.body))
			require.NoError(t, err)
			t.Cleanup(func() { _ = res.Body.Close() })
			got, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, res.StatusCode, string(got))
			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

func TestSendParcelPhotoNeedsAType(t *testing.T) {
	t.Parallel()

	c, err := NewClient("http://localhost:1")
	require.NoError(t, err)

	_, err = c.SendParcel(t.Context(), &SendParcelRequestOptions{Body: &Parcel{ID: "p1", Photo: new(runtime.NewFile([]byte("JPG"), "photo", ""))}})

	require.ErrorIs(t, err, runtime.ErrBodyValue)
	assert.ErrorContains(t, err, "photo: the file has no content type; the spec takes image/png, image/jpeg")
}

func TestSendParcelIDIsJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "A JSON string", id: `"p1"`, wantStatus: http.StatusOK},
		{name: "Plain text", id: "p1", wantStatus: http.StatusBadRequest},
	}

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			require.NoError(t, mw.WriteField("id", tc.id))
			require.NoError(t, mw.Close())
			res, err := http.Post(srv.URL+"/parcels", mw.FormDataContentType(), &body)
			require.NoError(t, err)
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, tc.wantStatus, res.StatusCode)
		})
	}
}

func TestPrintLabel(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	got, err := c.PrintLabel(t.Context(), &PrintLabelRequestOptions{Body: &Label{To: Address{City: "Rome"}, From: &Address{City: "Pisa"}}})
	require.NoError(t, err)
	assert.Equal(t, &Label{To: Address{City: "Rome", Zip: new("00000")}, From: &Address{City: "Pisa", Zip: new("00000")}}, got)

	req, err := c.PrintLabelRequest(t.Context(), &PrintLabelRequestOptions{Body: &Label{To: Address{City: "Rome"}, From: &Address{City: "Pisa"}}})
	require.NoError(t, err)
	sent, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, "from=%7B%22city%22%3A%22Pisa%22%7D&to=%7B%22city%22%3A%22Rome%22%7D", string(sent), "an object without a style goes as JSON")
}

func TestGetParcelAccept(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	opts := &GetParcelRequestOptions{PathParams: &GetParcelPathParams{ID: "p1"}}

	got, err := c.GetParcel(t.Context(), opts)
	require.NoError(t, err)
	assert.Equal(t, "application/json, text/csv, application/problem+json", *got.Note)

	csv := func(_ context.Context, req *http.Request) error {
		req.Header.Set("Accept", "text/csv")
		return nil
	}
	got, err = c.GetParcel(t.Context(), opts, csv)
	require.NoError(t, err)
	assert.Equal(t, "text/csv", *got.Note)
}
