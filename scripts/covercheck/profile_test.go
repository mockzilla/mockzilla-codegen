package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProfile(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", maxLineBytes+1)

	tests := []struct {
		name        string
		input       string
		want        []block
		wantErr     error
		wantErrText string
	}{
		{
			name: "Blocks are parsed and sorted by file and position",
			input: "mode: atomic\n" +
				"example.com/m/b.go:3.1,4.2 1 0\n" +
				"example.com/m/a.go:9.1,9.20 2 5\n" +
				"example.com/m/a.go:1.1,2.2 1 1\n",
			want: []block{
				{file: "example.com/m/a.go", startLine: 1, startCol: 1, endLine: 2, endCol: 2, stmts: 1, count: 1},
				{file: "example.com/m/a.go", startLine: 9, startCol: 1, endLine: 9, endCol: 20, stmts: 2, count: 5},
				{file: "example.com/m/b.go", startLine: 3, startCol: 1, endLine: 4, endCol: 2, stmts: 1, count: 0},
			},
		},
		{
			name: "Duplicate blocks keep the highest count",
			input: "mode: set\n" +
				"example.com/m/a.go:1.1,2.2 1 0\n" +
				"\n" +
				"example.com/m/a.go:1.1,2.2 1 3\n",
			want: []block{
				{file: "example.com/m/a.go", startLine: 1, startCol: 1, endLine: 2, endCol: 2, stmts: 1, count: 3},
			},
		},
		{
			name: "Blocks with the same start are ordered by end",
			input: "mode: set\n" +
				"example.com/m/a.go:1.1,5.2 1 0\n" +
				"example.com/m/a.go:1.1,3.2 1 0\n" +
				"example.com/m/a.go:1.1,3.1 1 0\n",
			want: []block{
				{file: "example.com/m/a.go", startLine: 1, startCol: 1, endLine: 3, endCol: 1, stmts: 1},
				{file: "example.com/m/a.go", startLine: 1, startCol: 1, endLine: 3, endCol: 2, stmts: 1},
				{file: "example.com/m/a.go", startLine: 1, startCol: 1, endLine: 5, endCol: 2, stmts: 1},
			},
		},
		{
			name:    "Empty input has no mode line",
			input:   "",
			wantErr: errNoMode,
		},
		{
			name:    "First line that is not a mode line is rejected",
			input:   "example.com/m/a.go:1.1,2.2 1 0\n",
			wantErr: errNoMode,
		},
		{
			name:    "Line without a colon is rejected",
			input:   "mode: set\nnot a block\n",
			wantErr: errBadLine,
		},
		{
			name:    "Line with bad numbers is rejected",
			input:   "mode: set\nexample.com/m/a.go:1.x,2.2 1 0\n",
			wantErr: errBadLine,
		},
		{
			name:        "Mode line longer than the buffer fails to read",
			input:       long,
			wantErrText: "read profile",
		},
		{
			name:        "Block line longer than the buffer fails to read",
			input:       "mode: set\n" + long,
			wantErrText: "read profile",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseProfile(strings.NewReader(tc.input))

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantErrText != "":
				require.ErrorContains(t, err, tc.wantErrText)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}
