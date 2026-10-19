package installlayout

import (
	"path/filepath"
	"testing"
)

func TestDirectoryFromSelectionUsesFixedMeowSystemLayout(t *testing.T) {
	tests := []struct {
		productID string
		name      string
	}{
		{productID: "meow-launcher", name: "Launcher"},
		{productID: "brain2-manager", name: "Brain2 Manager"},
		{productID: "dev-helper", name: "Helper"},
	}
	for _, test := range tests {
		got, err := DirectoryFromSelection(`E:\legacy\arbitrary`, test.productID)
		if err != nil {
			t.Fatalf("%s: %v", test.productID, err)
		}
		want := filepath.Join(`E:\`, SystemDirectoryName, test.name)
		if got != want {
			t.Fatalf("%s directory = %q, want %q", test.productID, got, want)
		}
		if !IsProductDirectory(got, test.productID) {
			t.Fatalf("%s canonical directory was rejected", test.productID)
		}
	}
}

func TestDirectoryFromSelectionRejectsUnknownProduct(t *testing.T) {
	if _, err := DirectoryFromSelection(`D:\anything`, "unknown-product"); err == nil {
		t.Fatal("unknown product unexpectedly received an install directory")
	}
}

func TestLegacyLauncherPathsConvergeOnOneDirectoryPerDrive(t *testing.T) {
	tests := []struct {
		legacy string
		want   string
	}{
		{legacy: `E:\launcher\Meow Launcher.exe`, want: `E:\Meow System\Launcher`},
		{legacy: `D:\launcher\Meow Launcher.exe`, want: `D:\Meow System\Launcher`},
	}
	for _, test := range tests {
		got, err := DirectoryFromSelection(test.legacy, "meow-launcher")
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("legacy %q resolved to %q, want %q", test.legacy, got, test.want)
		}
	}
}

func TestSystemRootFromSelectionPreservesUserChosenParent(t *testing.T) {
	tests := []struct {
		selection string
		want      string
	}{
		{selection: `E:\`, want: `E:\Meow System`},
		{selection: `D:\Apps`, want: `D:\Apps\Meow System`},
		{selection: `D:\Apps\Meow System`, want: `D:\Apps\Meow System`},
	}
	for _, test := range tests {
		got, err := SystemRootFromSelection(test.selection)
		if err != nil {
			t.Fatalf("%q: %v", test.selection, err)
		}
		if got != test.want {
			t.Fatalf("selection %q resolved to %q, want %q", test.selection, got, test.want)
		}
	}
}

func TestDirectoryForSystemRootUsesConfiguredRoot(t *testing.T) {
	root := `D:\Apps\Meow System`
	got, err := DirectoryForSystemRoot(root, "brain2-manager")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Brain2 Manager")
	if got != want {
		t.Fatalf("directory = %q, want %q", got, want)
	}
	if inferred, ok := SystemRootFromProductDirectory(got, "brain2-manager"); !ok || inferred != root {
		t.Fatalf("root inference = %q, %v; want %q, true", inferred, ok, root)
	}
}

func TestSystemRootFromSelectionRejectsNetworkShare(t *testing.T) {
	if _, err := SystemRootFromSelection(`\\server\share`); err == nil {
		t.Fatal("network share unexpectedly accepted as a Meow System root")
	}
}
