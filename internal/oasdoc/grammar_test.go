// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

type visited struct {
	ptr  string
	kind Kind
}

func TestVisit(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/grammar.yaml")
	require.NoError(t, err)
	d, err := Parse(data, "grammar.yaml")
	require.NoError(t, err)

	var got []visited
	Visit(d.Root(), "", KindDocument, func(ptr string, _ *yaml.Node, k Kind) bool {
		got = append(got, visited{ptr: ptr, kind: k})
		return ptr != "/components/pathItems/I"
	})
	assert.Equal(t, []visited{
		{ptr: "", kind: KindDocument},
		{ptr: "/paths", kind: KindPaths},
		{ptr: "/paths/~1a", kind: KindPathItem},
		{ptr: "/paths/~1a/parameters/0", kind: KindParameter},
		{ptr: "/paths/~1a/parameters/0/schema", kind: KindSchema},
		{ptr: "/paths/~1a/get", kind: KindOperation},
		{ptr: "/paths/~1a/get/parameters/0", kind: KindParameter},
		{ptr: "/paths/~1a/get/parameters/0/content/application~1json", kind: KindMediaType},
		{ptr: "/paths/~1a/get/parameters/0/content/application~1json/schema", kind: KindSchema},
		{ptr: "/paths/~1a/get/parameters/0/examples/one", kind: KindExample},
		{ptr: "/paths/~1a/get/requestBody", kind: KindRequestBody},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data", kind: KindMediaType},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data/schema", kind: KindSchema},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data/itemSchema", kind: KindSchema},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data/examples/e", kind: KindExample},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data/encoding/f", kind: KindEncoding},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data/encoding/f/headers/X-A", kind: KindHeader},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data/encoding/f/headers/X-A/schema", kind: KindSchema},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data/encoding/f/encoding/g", kind: KindEncoding},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data/prefixEncoding/0", kind: KindEncoding},
		{ptr: "/paths/~1a/get/requestBody/content/multipart~1form-data/itemEncoding", kind: KindEncoding},
		{ptr: "/paths/~1a/get/responses", kind: KindResponses},
		{ptr: "/paths/~1a/get/responses/200", kind: KindResponse},
		{ptr: "/paths/~1a/get/responses/200/headers/X-B", kind: KindHeader},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json", kind: KindMediaType},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/list", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/list/items", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/tuple", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/tuple/items/0", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/tuple/items/1", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/map", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/typed", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/typed/additionalProperties", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/x-prop", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/properties/x-prop/not", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/allOf/0", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/content/application~1json/schema/$defs/D", kind: KindSchema},
		{ptr: "/paths/~1a/get/responses/200/links/l", kind: KindLink},
		{ptr: "/paths/~1a/get/callbacks/cb", kind: KindCallback},
		{ptr: "/paths/~1a/get/callbacks/cb/{$request.body#~1url}", kind: KindPathItem},
		{ptr: "/paths/~1a/get/callbacks/cb/{$request.body#~1url}/post", kind: KindOperation},
		{ptr: "/paths/~1a/additionalOperations/COPY", kind: KindOperation},
		{ptr: "/webhooks/hook", kind: KindPathItem},
		{ptr: "/webhooks/hook/post", kind: KindOperation},
		{ptr: "/components", kind: KindComponents},
		{ptr: "/components/schemas/S", kind: KindSchema},
		{ptr: "/components/responses/R", kind: KindResponse},
		{ptr: "/components/parameters/P", kind: KindParameter},
		{ptr: "/components/examples/E", kind: KindExample},
		{ptr: "/components/requestBodies/B", kind: KindRequestBody},
		{ptr: "/components/headers/H", kind: KindHeader},
		{ptr: "/components/headers/H/schema", kind: KindSchema},
		{ptr: "/components/securitySchemes/K", kind: KindSecurityScheme},
		{ptr: "/components/links/L", kind: KindLink},
		{ptr: "/components/callbacks/C", kind: KindCallback},
		{ptr: "/components/pathItems/I", kind: KindPathItem},
		{ptr: "/components/mediaTypes/M", kind: KindMediaType},
	}, got)
}
