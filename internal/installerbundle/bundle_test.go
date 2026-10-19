package installerbundle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildOpenAndExtractSingleFileInstaller(t *testing.T) {
	root := t.TempDir()
	runner := filepath.Join(root, "runner.exe")
	payload := filepath.Join(root, "payload")
	output := filepath.Join(root, "release", "manager-installer.exe")
	if err := os.MkdirAll(filepath.Join(payload, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runner, []byte("MZ fake runner"))
	writeTestFile(t, filepath.Join(payload, "Brain2 Manager.exe"), []byte("manager executable"))
	writeTestFile(t, filepath.Join(payload, "assets", "config.json"), []byte("{}"))

	manifest, err := Build(BuildOptions{
		RunnerPath: runner, PayloadRoot: payload, OutputPath: output, ProductID: "brain2-manager", Version: "0.7.0-preview.1", ExecutableRelativePath: "Brain2 Manager.exe",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("manifest files = %+v", manifest.Files)
	}
	bundle, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	destination := filepath.Join(root, "extracted")
	files, err := bundle.Extract(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || string(readTestFile(t, filepath.Join(destination, "Brain2 Manager.exe"))) != "manager executable" {
		t.Fatalf("extracted = %+v", files)
	}
}

func TestExtractRejectsPayloadChangedAfterBundleWasBuilt(t *testing.T) {
	root := t.TempDir()
	runner := filepath.Join(root, "runner.exe")
	payload := filepath.Join(root, "payload")
	output := filepath.Join(root, "installer.exe")
	if err := os.MkdirAll(payload, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runner, []byte("MZ runner"))
	original := []byte("unique-payload-content")
	writeTestFile(t, filepath.Join(payload, "Dev Helper.exe"), original)
	if _, err := Build(BuildOptions{RunnerPath: runner, PayloadRoot: payload, OutputPath: output, ProductID: "dev-helper", Version: "1.0.0", ExecutableRelativePath: "Dev Helper.exe"}); err != nil {
		t.Fatal(err)
	}
	data := readTestFile(t, output)
	index := bytes.Index(data, original)
	if index < 0 {
		t.Fatal("stored payload not found")
	}
	data[index] ^= 0xff
	writeTestFile(t, output, data)
	bundle, err := Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	if _, err := bundle.Extract(filepath.Join(root, "extract")); err == nil {
		t.Fatal("tampered payload extracted")
	}
}

func TestBuildRejectsMissingOrEscapingEntrypoint(t *testing.T) {
	root := t.TempDir()
	runner := filepath.Join(root, "runner.exe")
	payload := filepath.Join(root, "payload")
	if err := os.MkdirAll(payload, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runner, []byte("runner"))
	writeTestFile(t, filepath.Join(payload, "app.exe"), []byte("app"))
	for _, entrypoint := range []string{"missing.exe", "../app.exe"} {
		if _, err := Build(BuildOptions{RunnerPath: runner, PayloadRoot: payload, OutputPath: filepath.Join(root, entrypoint+".installer"), ProductID: "dev-helper", Version: "1.0.0", ExecutableRelativePath: entrypoint}); err == nil {
			t.Fatalf("entrypoint %q accepted", entrypoint)
		}
	}
}

func writeTestFile(t *testing.T, filename string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o700); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
