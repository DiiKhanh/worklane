// Package arch holds an executable architecture rule: the OTP domain and application
// layers must not import infrastructure. If someone adds a Redis/GORM/Gin import to
// domain or app, this test fails - the hexagon boundary is checked by CI, not trust.
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
	// Substrings that must never appear in an import path of domain/ or app/.
	forbidden := []string{"redis", "gorm", "sarama", "gin", "resend", "/adapters/", "/pkg/platform/"}
	// Layers are relative to this test's directory (services/otp-api/internal/arch).
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
				continue // production files only; tests may import fakes freely
			}
			path := filepath.Join(dir, name)
			f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, imp := range f.Imports {
				for _, bad := range forbidden {
					if strings.Contains(imp.Path.Value, bad) {
						t.Errorf("%s imports forbidden %s (domain/app must stay infra-free)", path, imp.Path.Value)
					}
				}
			}
		}
	}
}
