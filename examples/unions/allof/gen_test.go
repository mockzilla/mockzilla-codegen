// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package allof

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Contact
	}{
		{name: "Shared fields and the email variant", data: `{"id":"1","address":"a@b.c"}`, want: Contact{ID: "1", Email: &Email{Address: "a@b.c"}}},
		{name: "Shared fields and the phone variant", data: `{"id":"2","number":"555"}`, want: Contact{ID: "2", Phone: &Phone{Number: "555"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Contact
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)
			require.NoError(t, got.Validate())

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}

func TestContactDecodeResets(t *testing.T) {
	t.Parallel()

	got := Contact{ID: "old", Phone: &Phone{Number: "1"}}
	require.NoError(t, json.Unmarshal([]byte(`{"id":"new","address":"x"}`), &got))

	assert.Equal(t, Contact{ID: "new", Email: &Email{Address: "x"}}, got)
}

func TestReminderJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want Reminder
	}{
		{name: "An email sent daily", data: `{"address":"a@b.c","time":"09:00"}`, want: Reminder{Email: &Email{Address: "a@b.c"}, Daily: &Daily{Time: "09:00"}}},
		{name: "A phone called weekly", data: `{"number":"555","weekday":1}`, want: Reminder{Phone: &Phone{Number: "555"}, Weekly: &Weekly{Weekday: 1}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Reminder
			require.NoError(t, json.Unmarshal([]byte(tc.data), &got))
			assert.Equal(t, tc.want, got)
			require.NoError(t, got.Validate())

			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.data, string(out))
		})
	}
}

func TestReminderDecodeNeedsEachUnion(t *testing.T) {
	t.Parallel()

	var got Reminder
	err := json.Unmarshal([]byte(`{"address":"a@b.c"}`), &got)

	require.EqualError(t, err, "no union variant matches for a JSON object: Daily needs time, Weekly needs weekday")
}

func TestReminderValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   Reminder
		wantErr string
	}{
		{
			name:    "A variant of each union is needed",
			value:   Reminder{Email: &Email{Address: "a@b.c"}},
			wantErr: "exactly one variant must be set, found 0",
		},
		{
			name:    "One variant of each union is taken",
			value:   Reminder{Email: &Email{Address: "a@b.c"}, Daily: &Daily{Time: "09:00"}, Weekly: &Weekly{Weekday: 1}},
			wantErr: "exactly one variant must be set, found 2",
		},
		{
			name:    "The variant that is set is checked",
			value:   Reminder{Phone: &Phone{Number: "555"}, Weekly: &Weekly{Weekday: 8}},
			wantErr: "weekday: must be at most 7",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.EqualError(t, tc.value.Validate(), tc.wantErr)
		})
	}
}
