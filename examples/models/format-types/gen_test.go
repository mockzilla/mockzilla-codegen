// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package formattypes

import (
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeviceJSON(t *testing.T) {
	t.Parallel()

	const data = `{"id":"7f1c2a9e-4b1d-4c3a-9a51-2f0d8e6b1c44","address":"10.0.0.7","peers":["0b3e5d2c-1f4a-4e8b-8c6d-9a7b5e3f2d10"]}`

	var d Device
	require.NoError(t, json.Unmarshal([]byte(data), &d))

	addr := netip.MustParseAddr("10.0.0.7")
	want := Device{
		ID:      uuid.MustParse("7f1c2a9e-4b1d-4c3a-9a51-2f0d8e6b1c44"),
		Address: &addr,
		Peers:   []uuid.UUID{uuid.MustParse("0b3e5d2c-1f4a-4e8b-8c6d-9a7b5e3f2d10")},
	}
	assert.Equal(t, want, d)

	out, err := json.Marshal(d)
	require.NoError(t, err)
	assert.JSONEq(t, data, string(out))
}

func TestDeviceBadID(t *testing.T) {
	t.Parallel()

	var d Device
	require.Error(t, json.Unmarshal([]byte(`{"id":"nope"}`), &d))
}

func TestDeviceLegacyIDKeepsItsType(t *testing.T) {
	t.Parallel()

	legacy := "nope"
	require.ErrorContains(t, Device{LegacyID: &legacy}.Validate(), "legacyId")
}
