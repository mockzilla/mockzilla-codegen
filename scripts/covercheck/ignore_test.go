package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadIgnore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content *string
		isDir   bool
		noPath  bool
		want    []string
		wantErr error
	}{
		{
			name:   "No path means no patterns",
			noPath: true,
		},
		{
			name: "Missing file means no patterns",
		},
		{
			name:    "Comments and blank lines are skipped",
			content: new("# generated\n\ncmd/codegen/main.go\n  internal/x/...  \n"),
			want:    []string{"cmd/codegen/main.go", "internal/x/..."},
		},
		{
			name:    "Malformed pattern is rejected",
			content: new("[\n"),
			wantErr: errBadPattern,
		},
		{
			name:  "Directory in place of a file fails to read",
			isDir: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file := filepath.Join(t.TempDir(), ".covignore")
			if tc.content != nil {
				require.NoError(t, os.WriteFile(file, []byte(*tc.content), 0o600))
			}
			if tc.isDir {
				require.NoError(t, os.Mkdir(file, 0o700))
			}
			if tc.noPath {
				file = ""
			}

			got, err := loadIgnore(file)

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.isDir:
				require.ErrorContains(t, err, "read ignore file")
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestIgnored(t *testing.T) {
	t.Parallel()

	patterns := []string{"cmd/*/main.go", "internal/gen/...", "doc.go"}

	tests := []struct {
		name string
		rel  string
		want bool
	}{
		{name: "Glob matches a file", rel: "cmd/codegen/main.go", want: true},
		{name: "Glob does not match another file", rel: "cmd/codegen/run.go", want: false},
		{name: "Tree pattern matches a file below it", rel: "internal/gen/models/x.go", want: true},
		{name: "Tree pattern matches the directory itself", rel: "internal/gen", want: true},
		{name: "Tree pattern does not match a sibling with the same prefix", rel: "internal/generator/x.go", want: false},
		{name: "Exact name matches", rel: "doc.go", want: true},
		{name: "Unlisted file is kept", rel: "version.go", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, ignored(tc.rel, patterns))
		})
	}
}
