package productregistry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpsertBrain2CreatesSharedDiscoveryRegistry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	for _, rel := range []string{
		filepath.Join(".brain2", "config.json"),
		filepath.Join(".brain2", "projects.json"),
		filepath.Join(".brain2", "version.json"),
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "registry.json")
	if err := UpsertBrain2(path, root, "brain2-test"); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Products) != 1 || registry.Products[0].ProjectsPath != filepath.Join(root, ".brain2", "projects.json") {
		t.Fatalf("unexpected registry: %#v", registry)
	}
	if err := UpsertBrain2(path, root, "brain2-test"); err != nil {
		t.Fatal(err)
	}
	registry, err = LoadFrom(path)
	if err != nil || len(registry.Products) != 1 {
		t.Fatalf("upsert duplicated instance: registry=%#v err=%v", registry, err)
	}
}
