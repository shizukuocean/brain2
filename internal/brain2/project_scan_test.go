package brain2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectScanRequiresAuthorizedCandidatesAndUserConfirmation(t *testing.T) {
	temp := t.TempDir()
	brainRoot := filepath.Join(temp, "Brain2")
	if err := Initialize(brainRoot); err != nil {
		t.Fatal(err)
	}
	scanRoot := filepath.Join(temp, "workspace")
	projectRoot := filepath.Join(scanRoot, "demo")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(projectRoot, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	task, err := CreateProjectScanTask(brainRoot, scanRoot, "codex")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := SubmitProjectCandidates(brainRoot, task, []ProjectCandidateInput{{
		Name: "Demo", RootPath: projectRoot, Confidence: 0.92,
		SuggestedGoal: "用于验证 Agent 扫描回填", Evidence: []string{"发现 go.mod", "发现 README.md"}, DetectedKind: "code_project",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Candidates) != 1 || registry.Candidates[0].Status != "pending" {
		t.Fatalf("candidate must wait for user confirmation: %#v", registry)
	}
	projects, err := LoadProjects(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects.Projects) != 0 {
		t.Fatalf("submission must not register a project: %#v", projects)
	}
	if err := SelectProjectCandidateForArchive(brainRoot, registry.Candidates[0].ID); err != nil {
		t.Fatal(err)
	}
	projects, err = LoadProjects(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects.Projects) != 0 {
		t.Fatalf("user selection must not register an incomplete project: %#v", projects)
	}
	if _, err := os.Stat(filepath.Join(brainRoot, "02 项目", "Demo")); !os.IsNotExist(err) {
		t.Fatalf("candidate selection must not create content-layer project notes: %v", err)
	}
	registry, err = LoadProjectCandidates(brainRoot)
	if err != nil || registry.Candidates[0].Status != "selected_for_archiving" {
		t.Fatalf("candidate was not selected for the archive workflow: registry=%#v err=%v", registry, err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatalf("source project content changed: data=%q err=%v", data, err)
	}

	outside := filepath.Join(temp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := SubmitProjectCandidates(brainRoot, task, []ProjectCandidateInput{{
		Name: "Outside", RootPath: outside, Confidence: 1, Evidence: []string{"outside"}, DetectedKind: "needs_verification",
	}}); err == nil {
		t.Fatal("expected a candidate outside the authorized scan root to be rejected")
	}
}

func TestProjectCandidateAdmissionRejectsNonProductThemes(t *testing.T) {
	temp := t.TempDir()
	root := filepath.Join(temp, "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	candidateRoot := filepath.Join(temp, "topic")
	if err := os.MkdirAll(candidateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	task, err := CreateProjectScanTask(root, "", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SubmitProjectCandidates(root, task, []ProjectCandidateInput{{
		Name: "论文主题", RootPath: candidateRoot, Confidence: 0.9,
		Evidence: []string{"存在资料目录"}, DetectedKind: "research_project",
	}}); err == nil {
		t.Fatal("non-product theme entered the project workflow")
	}
}

func TestBrain2FirstDiscoveryDoesNotRequireUserPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	task, err := CreateProjectScanTask(root, "", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if task.DiscoveryMode != "brain2_first" || task.ScanRoot != "" {
		t.Fatalf("unexpected discovery task: %#v", task)
	}
}

func TestProjectScanCanReportNoCandidates(t *testing.T) {
	temp := t.TempDir()
	brainRoot := filepath.Join(temp, "Brain2")
	if err := Initialize(brainRoot); err != nil {
		t.Fatal(err)
	}
	scanRoot := filepath.Join(temp, "empty")
	if err := os.MkdirAll(scanRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	task, err := CreateProjectScanTask(brainRoot, scanRoot, "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := SubmitProjectCandidates(brainRoot, task, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Candidates) != 0 {
		t.Fatalf("unexpected candidates: %#v", registry.Candidates)
	}
	stored, err := LoadProjectScanTask(brainRoot, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "submitted" {
		t.Fatalf("empty scan should still complete, got %q", stored.Status)
	}
}

func TestProjectScanKeepsIndependentProductsThatShareOneRepository(t *testing.T) {
	temp := t.TempDir()
	brainRoot := filepath.Join(temp, "Brain2")
	if err := Initialize(brainRoot); err != nil {
		t.Fatal(err)
	}
	repositoryRoot := filepath.Join(temp, "meow-launcher")
	if err := os.MkdirAll(repositoryRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	task, err := CreateProjectScanTask(brainRoot, "", "codex")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := SubmitProjectCandidates(brainRoot, task, []ProjectCandidateInput{
		{Name: "Meow Launcher", RootPath: repositoryRoot, Confidence: 0.99, Evidence: []string{"发现 Launcher 独立入口"}, DetectedKind: "product"},
		{Name: "Brain2 Manager", RootPath: repositoryRoot, Confidence: 0.98, Evidence: []string{"发现 Manager 独立构建物"}, DetectedKind: "product", SharedRepositoryProjectID: "meow-launcher"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Candidates) != 2 {
		t.Fatalf("shared repository products were collapsed: %#v", registry.Candidates)
	}
	managerID := ""
	for _, candidate := range registry.Candidates {
		if candidate.Name == "Brain2 Manager" {
			managerID = candidate.ID
			if candidate.SharedRepositoryProjectID != "meow-launcher" {
				t.Fatalf("shared repository relation missing: %#v", candidate)
			}
		}
	}
	if managerID == "" {
		t.Fatal("Brain2 Manager candidate missing")
	}
	registry, err = SubmitProjectCandidates(brainRoot, task, []ProjectCandidateInput{{
		Name: "Brain2 Manager", RootPath: repositoryRoot, Confidence: 1, Evidence: []string{"重新核验 Manager 独立构建物"}, DetectedKind: "product", SharedRepositoryProjectID: "meow-launcher",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Candidates) != 2 {
		t.Fatalf("Manager resubmission created a duplicate: %#v", registry.Candidates)
	}
	for _, candidate := range registry.Candidates {
		if candidate.Name == "Brain2 Manager" && candidate.ID != managerID {
			t.Fatalf("Manager candidate identity changed: before=%s after=%s", managerID, candidate.ID)
		}
	}
}
