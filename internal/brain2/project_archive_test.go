package brain2

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectArchiveCreatesHumanManifestAndVerifiedReceipt(t *testing.T) {
	temp := t.TempDir()
	brainRoot := filepath.Join(temp, "Brain2")
	if err := Initialize(brainRoot); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(temp, "Meow Launcher")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	discovery, err := CreateProjectScanTask(brainRoot, "", "codex")
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := SubmitProjectCandidates(brainRoot, discovery, []ProjectCandidateInput{{
		Name: "Meow Launcher", RootPath: projectRoot, SuggestedGoal: "统一产品入口",
		Confidence: 0.98, Evidence: []string{"Brain2 项目入口指向本机仓库", "发现 .git"}, DetectedKind: "product",
	}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateProjectArchiveTask(brainRoot, candidates.Candidates[0].ID, "codex")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := SubmitProjectArchive(brainRoot, task, ProjectArchiveInput{
		Name: "Meow Launcher", Goal: "统一安装、启动、升级与维护入口",
		EntityKind: "product", LifecycleStage: "active",
		RepositoryURL: "https://github.com/shizukuocean/meow-launcher.git", DefaultBranch: "main",
		AdditionalPaths: []ProjectArchivePathInput{{Path: `D:\MeowLauncher\meow-launcher`, DeviceID: "B", Status: "unknown"}},
		Evidence:        []string{"读取 Brain2 项目入口", "核验 git remote"}, NeedsVerification: []string{"B 机路径待 B 机复核"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "verified" || receipt.ProjectID == "" || len(receipt.ChangedPaths) < 4 {
		t.Fatalf("unexpected archive receipt: %#v", receipt)
	}
	projects, err := LoadProjects(brainRoot)
	if err != nil || len(projects.Projects) != 1 {
		t.Fatalf("archived project missing: projects=%#v err=%v", projects, err)
	}
	project := projects.Projects[0]
	if project.EntityKind != "product" || project.LifecycleStage != "active" || project.RepositoryURL != "https://github.com/shizukuocean/meow-launcher" {
		t.Fatalf("archive fields missing: %#v", project)
	}
	if project.ManifestPath == "" || hasProjectCapability(project, machineProjectCapability) {
		t.Fatalf("archive did not become a human-readable managed project: %#v", project)
	}
	manifest, err := os.ReadFile(filepath.Join(brainRoot, filepath.FromSlash(project.ManifestPath)))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"# 项目资产登记", "source_repository", "https://github.com/shizukuocean/meow-launcher", `D:\MeowLauncher\meow-launcher`} {
		if !strings.Contains(string(manifest), expected) {
			t.Fatalf("manifest missing %q:\n%s", expected, manifest)
		}
	}
	if _, err := VerifyProjectArchiveReceipt(brainRoot, task.TaskID); err != nil {
		t.Fatalf("receipt verification failed: %v", err)
	}
	candidates, err = LoadProjectCandidates(brainRoot)
	if err != nil || candidates.Candidates[0].Status != "archived" {
		t.Fatalf("candidate was not closed by archive receipt: candidates=%#v err=%v", candidates, err)
	}
}

func TestProjectArchiveRejectsUnselectedCandidateAndUnsafeRepository(t *testing.T) {
	temp := t.TempDir()
	root := filepath.Join(temp, "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(temp, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	discovery, err := CreateProjectScanTask(root, "", "codex")
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := SubmitProjectCandidates(root, discovery, []ProjectCandidateInput{{
		Name: "Demo", RootPath: projectRoot, Confidence: 1, Evidence: []string{"发现 go.mod"}, DetectedKind: "code_project",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SubmitProjectArchive(root, ProjectArchiveTask{
		TaskID: "manual", CandidateID: candidates.Candidates[0].ID, AllowedSourcePath: projectRoot, Status: "prepared",
	}, ProjectArchiveInput{Name: "Demo", Goal: "demo", EntityKind: "code_project", LifecycleStage: "active"}); err == nil {
		t.Fatal("unselected candidate was archived")
	}
	task, err := CreateProjectArchiveTask(root, candidates.Candidates[0].ID, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SubmitProjectArchive(root, task, ProjectArchiveInput{
		Name: "Demo", Goal: "demo", EntityKind: "code_project", LifecycleStage: "active",
		RepositoryURL: "https://token@github.com/example/demo", Evidence: []string{"核验 remote"},
	}); err == nil {
		t.Fatal("credential-bearing repository URL was accepted")
	}
}

func TestCreateProjectArchiveTasksBatchIsAtomicAndIndependent(t *testing.T) {
	temp := t.TempDir()
	root := filepath.Join(temp, "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	firstRoot := filepath.Join(temp, "First")
	secondRoot := filepath.Join(temp, "Second")
	for _, projectRoot := range []string{firstRoot, secondRoot} {
		if err := os.MkdirAll(projectRoot, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	discovery, err := CreateProjectScanTask(root, "", "codex")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := SubmitProjectCandidates(root, discovery, []ProjectCandidateInput{
		{Name: "First", RootPath: firstRoot, Confidence: 0.9, Evidence: []string{"发现 go.mod"}, DetectedKind: "code_project"},
		{Name: "Second", RootPath: secondRoot, Confidence: 0.8, Evidence: []string{"发现 package.json"}, DetectedKind: "application"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := CreateProjectArchiveTasks(root, []string{registry.Candidates[0].ID, registry.Candidates[1].ID}, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].TaskID == tasks[1].TaskID || tasks[0].Nonce == tasks[1].Nonce {
		t.Fatalf("batch tasks are not independent: %#v", tasks)
	}
	registry, err = LoadProjectCandidates(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range registry.Candidates {
		if candidate.Status != "selected_for_archiving" {
			t.Fatalf("candidate was not selected: %#v", registry.Candidates)
		}
	}
	for _, task := range tasks {
		if _, err := LoadProjectArchiveTask(root, task.TaskID); err != nil {
			t.Fatalf("task %s was not written: %v", task.TaskID, err)
		}
	}

	before, err := LoadProjectCandidates(root)
	if err != nil {
		t.Fatal(err)
	}
	beforeTasks, err := filepath.Glob(filepath.Join(root, ".brain2", "runtime", "project-archives", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateProjectArchiveTasks(root, []string{before.Candidates[0].ID, "missing"}, "codex"); err == nil {
		t.Fatal("mixed invalid candidate batch was accepted")
	}
	after, err := LoadProjectCandidates(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Candidates) != len(before.Candidates) {
		t.Fatalf("candidate count changed after rejected batch: before=%#v after=%#v", before, after)
	}
	for index := range before.Candidates {
		if after.Candidates[index].Status != before.Candidates[index].Status {
			t.Fatalf("candidate status changed after rejected batch: before=%#v after=%#v", before, after)
		}
	}
	afterTasks, err := filepath.Glob(filepath.Join(root, ".brain2", "runtime", "project-archives", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(afterTasks) != len(beforeTasks) {
		t.Fatalf("rejected batch left task files behind: before=%v after=%v", beforeTasks, afterTasks)
	}
	if _, err := CreateProjectArchiveTasks(root, []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u"}, "codex"); err == nil {
		t.Fatal("batch over 20 candidates was accepted")
	}
}

func TestMCPProjectArchiveRoundTrip(t *testing.T) {
	temp := t.TempDir()
	root := filepath.Join(temp, "Brain2")
	if err := Initialize(root); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(temp, "App")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	discovery, err := CreateProjectScanTask(root, "", "codex")
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := SubmitProjectCandidates(root, discovery, []ProjectCandidateInput{{
		Name: "App", RootPath: projectRoot, Confidence: 0.9, Evidence: []string{"发现 package.json"}, DetectedKind: "application",
	}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateProjectArchiveTask(root, candidates.Candidates[0].ID, "codex")
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "brain2_submit_project_archive", "arguments": map[string]any{
			"archive_task_id": task.TaskID, "archive_nonce": task.Nonce, "agent_id": task.AgentID,
			"archive": map[string]any{
				"name": "App", "goal": "桌面应用", "entity_kind": "application", "lifecycle_stage": "active",
				"evidence": []string{"核验 package.json"},
			},
		}},
	}
	line, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := ServeMCP(root, bytes.NewReader(append(line, '\n')), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"status":"verified"`) || !strings.Contains(output.String(), `"project_id":"app"`) {
		t.Fatalf("archive MCP response was not verified: %s", output.String())
	}
	if _, err := VerifyProjectArchiveReceipt(root, task.TaskID); err != nil {
		t.Fatalf("MCP receipt failed verification: %v", err)
	}
}

func TestProjectArchiveAllowsIndependentProductWithExplicitSharedRepository(t *testing.T) {
	temp := t.TempDir()
	brainRoot := filepath.Join(temp, "Brain2")
	if err := Initialize(brainRoot); err != nil {
		t.Fatal(err)
	}
	repositoryRoot := filepath.Join(temp, "meow-launcher")
	if err := os.MkdirAll(repositoryRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	archive := func(input ProjectCandidateInput, archiveInput ProjectArchiveInput) (ProjectArchiveReceipt, error) {
		discovery, err := CreateProjectScanTask(brainRoot, "", "codex")
		if err != nil {
			return ProjectArchiveReceipt{}, err
		}
		candidates, err := SubmitProjectCandidates(brainRoot, discovery, []ProjectCandidateInput{input})
		if err != nil {
			return ProjectArchiveReceipt{}, err
		}
		candidateID := ""
		for _, candidate := range candidates.Candidates {
			if candidate.Name == input.Name {
				candidateID = candidate.ID
				break
			}
		}
		task, err := CreateProjectArchiveTask(brainRoot, candidateID, "codex")
		if err != nil {
			return ProjectArchiveReceipt{}, err
		}
		return SubmitProjectArchive(brainRoot, task, archiveInput)
	}

	if _, err := archive(ProjectCandidateInput{
		Name: "Meow Launcher", RootPath: repositoryRoot, Confidence: 1, Evidence: []string{"Launcher 独立入口"}, DetectedKind: "product",
	}, ProjectArchiveInput{
		Name: "Meow Launcher", Goal: "产品安装与启动入口", EntityKind: "product", LifecycleStage: "active", Evidence: []string{"核验 Launcher 独立入口"},
	}); err != nil {
		t.Fatal(err)
	}
	receipt, err := archive(ProjectCandidateInput{
		Name: "Brain2 Manager", RootPath: repositoryRoot, Confidence: 1, Evidence: []string{"Manager 独立构建物"}, DetectedKind: "product", SharedRepositoryProjectID: "meow-launcher",
	}, ProjectArchiveInput{
		Name: "Brain2 Manager", Goal: "治理 Brain2 连接、项目与 Agent", EntityKind: "product", LifecycleStage: "active", SharedRepositoryProjectID: "meow-launcher", Evidence: []string{"核验 Manager 独立构建物"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ProjectID != "brain2-manager" {
		t.Fatalf("unexpected Manager project id: %#v", receipt)
	}
	projects, err := LoadProjects(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects.Projects) != 2 {
		t.Fatalf("shared products were not archived separately: %#v", projects.Projects)
	}
	for _, project := range projects.Projects {
		if project.ID != "brain2-manager" {
			continue
		}
		if project.SharedRepositoryProjectID != "meow-launcher" || !hasProjectCapability(project, "shared_repository") {
			t.Fatalf("Manager shared-repository identity missing: %#v", project)
		}
		manifest, readErr := os.ReadFile(filepath.Join(brainRoot, filepath.FromSlash(project.ManifestPath)))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(manifest), `shared_repository_project_id: "meow-launcher"`) || !strings.Contains(string(manifest), "独立产品，不是父子项目") {
			t.Fatalf("Manager manifest does not preserve repository semantics:\n%s", manifest)
		}
	}
}

func TestProjectArchiveRejectsMissingSharedRepositoryOwner(t *testing.T) {
	projects := ProjectRegistry{Projects: []Project{{ID: "meow-launcher"}}}
	err := validateSharedRepositoryRelation(projects, Project{
		ID: "brain2-manager", EntityKind: "product", SharedRepositoryProjectID: "missing-owner",
	}, `D:\MeowLauncher\meow-launcher`)
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("missing shared repository owner was accepted: %v", err)
	}
}
