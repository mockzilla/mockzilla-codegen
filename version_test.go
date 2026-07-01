package codegen

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	assert.Equal(t, devVersion, Version())
}

func TestVersionFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{
			name: "No build info gives dev",
			ok:   false,
			want: "dev",
		},
		{
			name: "Main module with a release version gives that version",
			info: &debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: "v1.2.3"}},
			ok:   true,
			want: "v1.2.3",
		},
		{
			name: "Main module built locally gives dev",
			info: &debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: "(devel)"}},
			ok:   true,
			want: "dev",
		},
		{
			name: "Dependency gives its required version",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/app"},
				Deps: []*debug.Module{
					{Path: "example.com/other", Version: "v0.1.0"},
					{Path: modulePath, Version: "v2.0.0"},
				},
			},
			ok:   true,
			want: "v2.0.0",
		},
		{
			name: "Dependency replaced by a local directory gives dev",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/app"},
				Deps: []*debug.Module{
					{Path: modulePath, Version: "v2.0.0", Replace: &debug.Module{Path: "../codegen"}},
				},
			},
			ok:   true,
			want: "dev",
		},
		{
			name: "Dependency replaced by another version gives the replacement",
			info: &debug.BuildInfo{
				Main: debug.Module{Path: "example.com/app"},
				Deps: []*debug.Module{
					{Path: modulePath, Version: "v2.0.0", Replace: &debug.Module{Path: "example.com/fork", Version: "v2.0.1"}},
				},
			},
			ok:   true,
			want: "v2.0.1",
		},
		{
			name: "Module missing from build info gives dev",
			info: &debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}},
			ok:   true,
			want: "dev",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, versionFrom(tc.info, tc.ok))
		})
	}
}
