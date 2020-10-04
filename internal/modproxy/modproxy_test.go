package modproxy

import (
	"fmt"
	"testing"
)

func TestLatest(t *testing.T) {
	mod, err := Latest("github.com/go-redis/redis")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(mod.Path, mod.Latest())
}

func TestQuery(t *testing.T) {
	mod, ok, err := Query("github.com/go-redis/redis")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		fmt.Println("not found")
		return
	}
	fmt.Println(mod.Path)
	for _, v := range mod.Versions {
		fmt.Println(v)
	}
}

func TestModule(t *testing.T) {
	tests := []struct {
		mod      *Module
		nextpath string
		latest   string
	}{
		{
			mod: &Module{
				Path: "github.com/go-redis/redis",
				Versions: []string{
					"v3.2.30+incompatible",
					"v5.1.2+incompatible",
					"v4.1.11+incompatible",
					"v4.1.3+incompatible",
					"v3.2.13+incompatible",
					"v3.2.22+incompatible",
					"v3.6.4+incompatible",
					"v6.2.3+incompatible",
					"v6.14.1+incompatible",
					"v4.2.3+incompatible",
					"v3.2.16+incompatible",
					"v4.0.1+incompatible",
					"v6.0.0+incompatible",
					"v6.8.2+incompatible",
				},
			},
			latest:   "v6.14.1+incompatible",
			nextpath: "github.com/go-redis/redis/v7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.mod.Path, func(t *testing.T) {
			t.Run("Latest", func(t *testing.T) {
				latest := tt.mod.Latest()
				if latest != tt.latest {
					t.Fatalf("wrong latest version, want %s, got %s", tt.latest, latest)
				}
			})
			t.Run("NextMajorPath", func(t *testing.T) {
				nextpath, ok := tt.mod.NextMajorPath()
				if !ok {
					t.Fatal("failed to get next major version")
				}
				if nextpath != tt.nextpath {
					t.Fatalf("wrong next path, want %s, got %s", tt.nextpath, nextpath)
				}
			})
		})
	}
}