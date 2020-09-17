package packages

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		path string
		pkg  *Package
	}{
		{
			path: "gotest.tools",
			pkg: &Package{
				PkgDir:    "",
				ModPrefix: "gotest.tools",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			pkg, err := Load(tt.path)
			assert.NilError(t, err)
			assert.DeepEqual(t, pkg, tt.pkg)
		})
	}
}
