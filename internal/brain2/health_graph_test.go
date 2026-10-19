package brain2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthReportsBrokenLinksAndIsolationAsEvidenceOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(root, "notes")
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(notes, "source.md")
	source := []byte("# Source\n\n[[missing-note]]\n")
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notes, "lonely.md"), []byte("# Lonely\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Check(root)
	if err != nil {
		t.Fatal(err)
	}
	foundBroken := false
	foundCandidate := false
	for _, check := range report.Checks {
		if check.Category == "broken_link" && strings.Contains(check.Name, "missing-note") {
			foundBroken = true
		}
		if check.Category == "isolation_candidate" && strings.HasSuffix(check.Path, "lonely.md") && check.Status == "info" {
			foundCandidate = true
		}
	}
	if !foundBroken || !foundCandidate {
		t.Fatalf("expected broken link and non-blocking isolation evidence: %#v", report.Checks)
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil || string(after) != string(source) {
		t.Fatalf("health check modified user content: data=%q err=%v", after, err)
	}
}
