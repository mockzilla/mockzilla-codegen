// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package contract

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service implements PetsInterface, which is what a user does.
type service struct{}

func (service) ListPets(context.Context, *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {
	return NewListPetsResponseData200(ListPetsResponse200{{Name: "Rex"}}).
		WithTypedHeaders200(ListPetsResponse200Headers{XTotalCount: 1, XNext: new("p2")}), nil
}

func (service) CreatePet(context.Context, *CreatePetServiceRequestOptions) (*CreatePetResponseData, error) {
	return NewCreatePetResponseData4XX(http.StatusUnprocessableEntity, &Problem{Detail: new("no")}), nil
}

func (service) DeletePet(context.Context, *DeletePetServiceRequestOptions) (*DeletePetResponseData, error) {
	return NewDeletePetResponseData().WithStatus(http.StatusAccepted), nil
}

func (service) Upload(context.Context, *UploadServiceRequestOptions) (*UploadResponseData, error) {
	return NewUploadResponseData([]byte("raw")).WithHeaders(http.Header{"X-A": {"1"}}), nil
}

func (service) GetPing(context.Context, *GetPingServiceRequestOptions) (*GetPingResponseData, error) {
	return NewGetPingResponseData(), nil
}

func TestResponseData(t *testing.T) {
	t.Parallel()

	var svc PetsInterface = service{}
	ctx := context.Background()

	list, err := svc.ListPets(ctx, &ListPetsServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, 200, list.Status)
	assert.Equal(t, http.Header{"X-Total-Count": {"1"}, "X-Next": {"p2"}}, list.Headers)
	assert.Equal(t, ListPetsResponse200{{Name: "Rex"}}, list.Payload())
	assert.Equal(t, "application/json", list.ContentType())

	created, err := svc.CreatePet(ctx, &CreatePetServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, 422, created.Status)

	deleted, err := svc.DeletePet(ctx, &DeletePetServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, 202, deleted.Status)
	assert.Nil(t, deleted.Payload())

	uploaded, err := svc.Upload(ctx, &UploadServiceRequestOptions{Body: "text"})
	require.NoError(t, err)
	assert.Equal(t, "application/octet-stream", uploaded.ContentType())
	assert.Equal(t, http.Header{"X-A": {"1"}}, uploaded.Headers)
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&ListPetsServiceRequestOptions{}).Validate())
	require.EqualError(t, (&ListPetsServiceRequestOptions{Query: &ListPetsQuery{Limit: new(0)}}).Validate(), "query.limit: must be at least 1")
	require.EqualError(t, (&CreatePetServiceRequestOptions{BodyJSON: &Pet{}}).Validate(), "body.name: must be at least 1 characters long")
	require.NoError(t, (&GetPingServiceRequestOptions{}).Validate())
}
