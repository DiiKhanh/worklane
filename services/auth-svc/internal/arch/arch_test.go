// Package arch holds an executable architecture rule: the auth domain and application
// layers must not import infrastructure. The hexagon boundary is checked by CI, not trust.
package arch

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDomainAndAppAreInfraFree(t *testing.T) {
	forbidden := []string{"redis", "gorm", "sarama", "gin", "/adapters/", "/pkg/platform/"}
	layers := []string{"../domain", "../app"}

	fset := token.NewFileSet()
	for _, dir := range layers {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for _, imp := range f.Imports {
				for _, bad := range forbidden {
					if strings.Contains(imp.Path.Value, bad) {
						t.Errorf("%s imports forbidden %s", name, imp.Path.Value)
					}
				}
			}
		}
	}
}
