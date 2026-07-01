package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModulePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content *string
		want    string
		wantErr error
	}{
		{
			name:    "Module line gives the module path",
			content: new("// comment\nmodule example.com/m\n\ngo 1.26.0\n"),
			want:    "example.com/m",
		},
		{
			name:    "Quoted module path is unquoted",
			content: new(`module "example.com/quoted"` + "\n"),
			want:    "example.com/quoted",
		},
		{
			name:    "File without a module line is rejected",
			content: new("go 1.26.0\n"),
			wantErr: errNoModule,
		},
		{
			name: "Missing file fails to read",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file := filepath.Join(t.TempDir(), "go.mod")
			if tc.content != nil {
				require.NoError(t, os.WriteFile(file, []byte(*tc.content), 0o600))
			}

			got, err := modulePath(file)

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.content == nil:
				require.ErrorContains(t, err, "read go.mod")
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}
