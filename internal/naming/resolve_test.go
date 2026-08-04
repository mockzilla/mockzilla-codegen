// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package naming

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
)

func TestRenameDiagnostic(t *testing.T) {
	t.Parallel()

	origin := diag.Origin{File: "openapi.yaml", Line: 12, Col: 5}
	tests := []struct {
		name   string
		rename Rename
		want   diag.Diagnostic
	}{
		{
			name:   "Name held by another request",
			rename: Rename{ID: "/b", From: "Pet", To: "PetResponse", Holder: "/a", Rank: RankComponent, Origin: origin},
			want: diag.Diagnostic{
				Severity: diag.Info,
				Code:     diag.CodeNameClash,
				Pointer:  "/b",
				Origin:   origin,
				Message:  `"Pet" is used by /a, renamed to "PetResponse"`,
			},
		},
		{
			name:   "Reserved name",
			rename: Rename{ID: "/b", From: "Client", To: "Client2", Rank: RankComponentSchema},
			want: diag.Diagnostic{
				Severity: diag.Info,
				Code:     diag.CodeNameClash,
				Pointer:  "/b",
				Message:  `"Client" is reserved, renamed to "Client2"`,
			},
		},
		{
			name:   "Lost x-go-name is a warning",
			rename: Rename{ID: "/b", From: "Pet", To: "Pet2", Holder: "/a", Rank: RankGoName},
			want: diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeNameClash,
				Pointer:  "/b",
				Message:  `"Pet" is used by /a, renamed to "Pet2"`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.rename.Diagnostic())
		})
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		reserved []string
		reqs     []Request
		want     Result
	}{
		{
			name: "No requests",
			want: Result{Names: map[string]string{}, Ordered: []Assignment{}},
		},
		{
			name: "Distinct names are kept",
			reqs: []Request{{ID: "/a", Want: "Pet"}, {ID: "/b", Want: "Owner", Order: 1}},
			want: Result{
				Names:   map[string]string{"/a": "Pet", "/b": "Owner"},
				Ordered: []Assignment{{ID: "/a", Name: "Pet"}, {ID: "/b", Name: "Owner"}},
			},
		},
		{
			name: "Names that differ only by case clash",
			reqs: []Request{{ID: "/a", Want: "PetId"}, {ID: "/b", Want: "PetID", Order: 1}},
			want: Result{
				Names:   map[string]string{"/a": "PetId", "/b": "PetID2"},
				Ordered: []Assignment{{ID: "/a", Name: "PetId"}, {ID: "/b", Name: "PetID2"}},
				Renames: []Rename{{ID: "/b", From: "PetID", To: "PetID2", Holder: "/a"}},
			},
		},
		{
			name: "Component keeps the name even when the inline type comes first in the spec",
			reqs: []Request{
				{ID: "/paths/~1pets/get/pet", Want: "Pet", Rank: RankInline},
				{ID: "/components/schemas/Pet", Want: "Pet", Rank: RankComponentSchema, Order: 9},
			},
			want: Result{
				Names:   map[string]string{"/components/schemas/Pet": "Pet", "/paths/~1pets/get/pet": "Pet2"},
				Ordered: []Assignment{{ID: "/components/schemas/Pet", Name: "Pet"}, {ID: "/paths/~1pets/get/pet", Name: "Pet2"}},
				Renames: []Rename{{ID: "/paths/~1pets/get/pet", From: "Pet", To: "Pet2", Holder: "/components/schemas/Pet"}},
			},
		},
		{
			name: "Every rank in order",
			reqs: []Request{
				{ID: "/a", Want: "Pet", Rank: RankInline},
				{ID: "/b", Want: "Pet", Rank: RankOperation},
				{ID: "/c", Want: "Pet", Rank: RankComponent},
				{ID: "/d", Want: "Pet", Rank: RankComponentSchema},
				{ID: "/e", Want: "Pet", Rank: RankGoName},
			},
			want: Result{
				Names: map[string]string{"/e": "Pet", "/d": "Pet2", "/c": "Pet3", "/b": "Pet4", "/a": "Pet5"},
				Ordered: []Assignment{
					{ID: "/e", Name: "Pet"},
					{ID: "/d", Name: "Pet2"},
					{ID: "/c", Name: "Pet3"},
					{ID: "/b", Name: "Pet4"},
					{ID: "/a", Name: "Pet5"},
				},
				Renames: []Rename{
					{ID: "/d", From: "Pet", To: "Pet2", Holder: "/e", Rank: RankComponentSchema},
					{ID: "/c", From: "Pet", To: "Pet3", Holder: "/e", Rank: RankComponent},
					{ID: "/b", From: "Pet", To: "Pet4", Holder: "/e", Rank: RankOperation},
					{ID: "/a", From: "Pet", To: "Pet5", Holder: "/e", Rank: RankInline},
				},
			},
		},
		{
			name: "Equal rank and order fall back to the ID",
			reqs: []Request{{ID: "/b", Want: "Pet"}, {ID: "/a", Want: "Pet"}},
			want: Result{
				Names:   map[string]string{"/a": "Pet", "/b": "Pet2"},
				Ordered: []Assignment{{ID: "/a", Name: "Pet"}, {ID: "/b", Name: "Pet2"}},
				Renames: []Rename{{ID: "/b", From: "Pet", To: "Pet2", Holder: "/a"}},
			},
		},
		{
			name: "Fallback comes before a number",
			reqs: []Request{
				{ID: "/components/schemas/Pet", Want: "Pet", Rank: RankComponentSchema},
				{ID: "/components/responses/Pet", Want: "Pet", Fallback: "PetResponse", Rank: RankComponent},
			},
			want: Result{
				Names:   map[string]string{"/components/schemas/Pet": "Pet", "/components/responses/Pet": "PetResponse"},
				Ordered: []Assignment{{ID: "/components/schemas/Pet", Name: "Pet"}, {ID: "/components/responses/Pet", Name: "PetResponse"}},
				Renames: []Rename{{ID: "/components/responses/Pet", From: "Pet", To: "PetResponse", Holder: "/components/schemas/Pet", Rank: RankComponent}},
			},
		},
		{
			name: "Taken fallback gets a number",
			reqs: []Request{
				{ID: "/a", Want: "Pet", Rank: RankComponentSchema},
				{ID: "/b", Want: "PetResponse", Rank: RankComponentSchema, Order: 1},
				{ID: "/c", Want: "Pet", Fallback: "PetResponse", Rank: RankComponent},
			},
			want: Result{
				Names: map[string]string{"/a": "Pet", "/b": "PetResponse", "/c": "PetResponse2"},
				Ordered: []Assignment{
					{ID: "/a", Name: "Pet"}, {ID: "/b", Name: "PetResponse"}, {ID: "/c", Name: "PetResponse2"},
				},
				Renames: []Rename{{ID: "/c", From: "Pet", To: "PetResponse2", Holder: "/a", Rank: RankComponent}},
			},
		},
		{
			name: "Numbers skip names already taken",
			reqs: []Request{
				{ID: "/a", Want: "Pet", Rank: RankComponentSchema},
				{ID: "/b", Want: "Pet2", Rank: RankComponentSchema, Order: 1},
				{ID: "/c", Want: "Pet"},
				{ID: "/d", Want: "Pet", Order: 1},
			},
			want: Result{
				Names: map[string]string{"/a": "Pet", "/b": "Pet2", "/c": "Pet3", "/d": "Pet4"},
				Ordered: []Assignment{
					{ID: "/a", Name: "Pet"}, {ID: "/b", Name: "Pet2"}, {ID: "/c", Name: "Pet3"}, {ID: "/d", Name: "Pet4"},
				},
				Renames: []Rename{
					{ID: "/c", From: "Pet", To: "Pet3", Holder: "/a"},
					{ID: "/d", From: "Pet", To: "Pet4", Holder: "/a"},
				},
			},
		},
		{
			name:     "Reserved generator name",
			reserved: []string{"Client", "NewRouter"},
			reqs:     []Request{{ID: "/components/schemas/client", Want: "Client", Fallback: "ClientSchema", Rank: RankComponentSchema}},
			want: Result{
				Names:   map[string]string{"/components/schemas/client": "ClientSchema"},
				Ordered: []Assignment{{ID: "/components/schemas/client", Name: "ClientSchema"}},
				Renames: []Rename{{ID: "/components/schemas/client", From: "Client", To: "ClientSchema", Rank: RankComponentSchema}},
			},
		},
		{
			name:     "Reserved names clash ignoring case",
			reserved: []string{"client"},
			reqs:     []Request{{ID: "/a", Want: "Client"}},
			want: Result{
				Names:   map[string]string{"/a": "Client2"},
				Ordered: []Assignment{{ID: "/a", Name: "Client2"}},
				Renames: []Rename{{ID: "/a", From: "Client", To: "Client2"}},
			},
		},
		{
			name:     "Field clashes with a method of its struct",
			reserved: []string{"Validate", "MarshalJSON", "UnmarshalJSON", "Error", "String", "Masked", "LogValue"},
			reqs: []Request{
				{ID: "/components/schemas/Pet/properties/validate", Want: "Validate", Fallback: "ValidateField"},
				{ID: "/components/schemas/Pet/properties/name", Want: "Name", Order: 1},
			},
			want: Result{
				Names: map[string]string{
					"/components/schemas/Pet/properties/validate": "ValidateField",
					"/components/schemas/Pet/properties/name":     "Name",
				},
				Ordered: []Assignment{
					{ID: "/components/schemas/Pet/properties/validate", Name: "ValidateField"},
					{ID: "/components/schemas/Pet/properties/name", Name: "Name"},
				},
				Renames: []Rename{{ID: "/components/schemas/Pet/properties/validate", From: "Validate", To: "ValidateField"}},
			},
		},
		{
			name:     "Enum constant clashes with a type resolved before it",
			reserved: []string{"Status", "StatusActive"},
			reqs: []Request{
				{ID: "/components/schemas/Status/enum/0", Want: "StatusActive"},
				{ID: "/components/schemas/Status/enum/1", Want: "StatusDone", Order: 1},
			},
			want: Result{
				Names: map[string]string{
					"/components/schemas/Status/enum/0": "StatusActive2",
					"/components/schemas/Status/enum/1": "StatusDone",
				},
				Ordered: []Assignment{
					{ID: "/components/schemas/Status/enum/0", Name: "StatusActive2"},
					{ID: "/components/schemas/Status/enum/1", Name: "StatusDone"},
				},
				Renames: []Rename{{ID: "/components/schemas/Status/enum/0", From: "StatusActive", To: "StatusActive2"}},
			},
		},
		{
			name: "Origin is carried to the rename",
			reqs: []Request{
				{ID: "/a", Want: "Pet"},
				{ID: "/b", Want: "Pet", Order: 1, Origin: diag.Origin{File: "pet.yaml", Line: 3, Col: 7}},
			},
			want: Result{
				Names:   map[string]string{"/a": "Pet", "/b": "Pet2"},
				Ordered: []Assignment{{ID: "/a", Name: "Pet"}, {ID: "/b", Name: "Pet2"}},
				Renames: []Rename{{ID: "/b", From: "Pet", To: "Pet2", Holder: "/a", Origin: diag.Origin{File: "pet.yaml", Line: 3, Col: 7}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Resolve(tt.reserved, tt.reqs))
		})
	}
}

func TestResolveIsDeterministic(t *testing.T) {
	t.Parallel()

	reserved := []string{"Client", "Validate"}
	var reqs []Request
	for i, want := range []string{"Pet", "pet", "PET", "Client", "Owner", "Pet2", "Validate", "Owner"} {
		for rank := RankInline; rank <= RankGoName; rank++ {
			reqs = append(reqs, Request{
				ID:       "/" + strconv.Itoa(i) + "/" + strconv.Itoa(int(rank)),
				Want:     want,
				Fallback: want + "Schema",
				Rank:     rank,
				Order:    i % 3,
			})
		}
	}
	want := Resolve(reserved, reqs)

	rng := rand.New(rand.NewPCG(1, 2))
	for range 100 {
		shuffled := slices.Clone(reqs)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		assert.Equal(t, want, Resolve(reserved, shuffled))
	}
}
