// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package enums

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValues(t *testing.T) {
	t.Parallel()

	values := StatusValues()
	assert.Equal(t, []Status{StatusPlaced, StatusInTransit, StatusDelivered}, values)

	values[0] = StatusDelivered
	assert.Equal(t, StatusPlaced, StatusValues()[0])
	assert.Equal(t, []OrderChannel{OrderChannelWeb, OrderChannelPhone}, OrderChannelValues())
}
