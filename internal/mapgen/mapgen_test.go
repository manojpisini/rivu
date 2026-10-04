package mapgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manojpisini/rivu/internal/registry"
)

func TestBuildWritesStackAndFallback(t *testing.T) {
	tests := []struct {
		name  string
		stack []string
		want  string
	}{
		{"detected stack", []string{"go", "sqlite"}, "go, sqlite"},
		{"empty stack fallback", nil, "Not detected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := registry.Project{Name: "demo", Path: t.TempDir(), Stack: tt.stack}
			if err := os.WriteFile(filepath.Join(p.Path, "main.go"), []byte("package main"), 0644); err != nil {
				t.Fatal(err)
			}
			for _, dir := range []string{"node_modules", ".git"} {
				if err := os.MkdirAll(filepath.Join(p.Path, dir), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := Build(p); err != nil {
				t.Fatalf("Build: %v", err)
			}
			b, err := os.ReadFile(filepath.Join(p.Path, ".metadata", "agent", "PROJECT_MAP.md"))
			if err != nil {
				t.Fatalf("read map: %v", err)
			}
			body := string(b)
			if !strings.Contains(body, "## Stack\n"+tt.want) {
				t.Errorf("map body missing stack %q", tt.want)
			}
			if !strings.Contains(body, "`main.go`") {
				t.Error("map does not list main.go")
			}
			if strings.Contains(body, "node_modules/") || strings.Contains(body, ".git/") {
				t.Error("ignored directory leaked into map")
			}
			agents := filepath.Join(p.Path, ".metadata", "agent", "AGENTS.md")
			if _, err := os.Stat(agents); err != nil {
				t.Errorf("AGENTS.md missing: %v", err)
			}
		})
	}
}
