package brain2

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const projectArchiveSchemaVersion = "0.1"

type ProjectArchiveTask struct {
	SchemaVersion     string `json:"schema_version"`
	TaskID            string `json:"task_id"`
	Nonce             string `json:"nonce"`
	AgentID           string `json:"agent_id"`
	WorkspaceID       string `json:"workspace_id"`
	CandidateID       string `json:"candidate_id"`
	AllowedSourcePath string `json:"allowed_source_path"`
	Status            string `json:"status"`
	CreatedAt         string `json:"created_at"`
	CompletedAt       string `json:"completed_at,omitempty"`
}

type ProjectArchivePathInput struct {
	Path     string `json:"path"`
	DeviceID string `json:"device_id"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

type ProjectArchiveInput struct {
	Name                      string                    `json:"name"`
	Goal                      string                    `json:"goal"`
	EntityKind                string                    `json:"entity_kind"`
	LifecycleStage            string                    `json:"lifecycle_stage"`
	Brain2Entry               string                    `json:"brain2_entry"`
	ParentProjectID           string                    `json:"parent_project_id"`
	SharedRepositoryProjectID string                    `json:"shared_repository_project_id,omitempty"`
	RepositoryURL             string                    `json:"repository_url"`
	DefaultBranch             string                    `json:"default_branch"`
	AdditionalPaths           []ProjectArchivePathInput `json:"additional_paths"`
	Evidence                  []string                  `json:"evidence"`
	NeedsVerification         []string                  `json:"needs_verification"`
}

type ProjectArchiveReceipt struct {
	SchemaVersion     string   `json:"schema_version"`
	TaskID            string   `json:"task_id"`
	CandidateID       string   `json:"candidate_id"`
	ProjectID         string   `json:"project_id"`
	Status            string   `json:"status"`
	ChangedPaths      []string `json:"changed_paths"`
	Evidence          []string `json:"evidence"`
	NeedsVerification []string `json:"needs_verification"`
	CompletedAt       string   `json:"completed_at"`
}

func CreateProjectArchiveTask(root, candidateID, agentID string) (ProjectArchiveTask, error) {
	tasks, err := CreateProjectArchiveTasks(root, []string{candidateID}, agentID)
	if err != nil {
		return ProjectArchiveTask{}, err
	}
	return tasks[0], nil
}

func CreateProjectArchiveTasks(root string, candidateIDs []string, agentID string) ([]ProjectArchiveTask, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return nil, err
	}
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return nil, errors.New("请选择负责建档的 Agent")
	}
	if len(candidateIDs) == 0 {
		return nil, errors.New("请至少选择一个项目候选")
	}
	if len(candidateIDs) > 20 {
		return nil, errors.New("单次批量建档最多选择 20 个候选")
	}
	registry, err := LoadProjectCandidates(root)
	if err != nil {
		return nil, err
	}
	indices := make([]int, 0, len(candidateIDs))
	seen := map[string]bool{}
	for _, candidateID := range candidateIDs {
		candidateID = strings.TrimSpace(candidateID)
		if candidateID == "" || seen[candidateID] {
			return nil, errors.New("批量建档候选包含空值或重复项")
		}
		seen[candidateID] = true
		candidateIndex := -1
		for index := range registry.Candidates {
			if registry.Candidates[index].ID == candidateID {
				candidateIndex = index
				break
			}
		}
		if candidateIndex < 0 {
			return nil, fmt.Errorf("项目候选不存在: %s", candidateID)
		}
		candidate := registry.Candidates[candidateIndex]
		if !allowedProjectEntityKind(candidate.DetectedKind) {
			return nil, fmt.Errorf("候选 %s 不属于代码、应用或产品项目", candidate.Name)
		}
		if candidate.Status != "pending" && candidate.Status != "selected_for_archiving" {
			return nil, fmt.Errorf("候选 %s 当前不能加入建档", candidate.Name)
		}
		indices = append(indices, candidateIndex)
	}
	workspaceID, err := workspaceInstanceID(root)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tasks := make([]ProjectArchiveTask, 0, len(indices))
	for _, candidateIndex := range indices {
		candidate := registry.Candidates[candidateIndex]
		taskID, randomErr := randomHex(12)
		if randomErr != nil {
			return nil, randomErr
		}
		nonce, randomErr := randomHex(24)
		if randomErr != nil {
			return nil, randomErr
		}
		tasks = append(tasks, ProjectArchiveTask{
			SchemaVersion: projectArchiveSchemaVersion, TaskID: taskID, Nonce: nonce,
			AgentID: agentID, WorkspaceID: workspaceID, CandidateID: candidate.ID,
			AllowedSourcePath: filepath.Clean(candidate.RootPath), Status: "prepared", CreatedAt: now,
		})
	}
	written := make([]string, 0, len(tasks))
	for _, task := range tasks {
		path := projectArchiveTaskPath(root, task.TaskID)
		if err := writeJSON(path, task); err != nil {
			for _, rollbackPath := range written {
				_ = os.Remove(rollbackPath)
			}
			return nil, err
		}
		written = append(written, path)
	}
	for _, candidateIndex := range indices {
		registry.Candidates[candidateIndex].Status = "selected_for_archiving"
		registry.Candidates[candidateIndex].ParentProjectID = ""
	}
	if err := writeJSON(filepath.Join(root, ".brain2", "project-candidates.json"), registry); err != nil {
		for _, rollbackPath := range written {
			_ = os.Remove(rollbackPath)
		}
		return nil, err
	}
	return tasks, nil
}

func LoadProjectArchiveTask(root, taskID string) (ProjectArchiveTask, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ProjectArchiveTask{}, err
	}
	if !validToken(taskID) {
		return ProjectArchiveTask{}, errors.New("无效的项目建档任务")
	}
	var task ProjectArchiveTask
	data, err := os.ReadFile(projectArchiveTaskPath(root, taskID))
	if err != nil {
		return ProjectArchiveTask{}, errors.New("项目建档任务不存在")
	}
	if err := json.Unmarshal(data, &task); err != nil {
		return ProjectArchiveTask{}, fmt.Errorf("项目建档任务损坏: %w", err)
	}
	return task, nil
}

func SubmitProjectArchive(root string, task ProjectArchiveTask, input ProjectArchiveInput) (ProjectArchiveReceipt, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ProjectArchiveReceipt{}, err
	}
	if task.Status != "prepared" {
		return ProjectArchiveReceipt{}, errors.New("项目建档任务当前不可提交")
	}
	candidates, err := LoadProjectCandidates(root)
	if err != nil {
		return ProjectArchiveReceipt{}, err
	}
	candidateIndex := -1
	for index := range candidates.Candidates {
		if candidates.Candidates[index].ID == task.CandidateID {
			candidateIndex = index
			break
		}
	}
	if candidateIndex < 0 || candidates.Candidates[candidateIndex].Status != "selected_for_archiving" {
		return ProjectArchiveReceipt{}, errors.New("项目候选未获用户建档授权")
	}
	candidate := candidates.Candidates[candidateIndex]
	if !strings.EqualFold(filepath.Clean(candidate.RootPath), filepath.Clean(task.AllowedSourcePath)) {
		return ProjectArchiveReceipt{}, errors.New("建档任务授权路径与候选不一致")
	}
	project, entryPath, entryExisted, err := validateProjectArchiveInput(root, candidate, input)
	if err != nil {
		return ProjectArchiveReceipt{}, err
	}
	projects, err := LoadProjects(root)
	if err != nil {
		return ProjectArchiveReceipt{}, err
	}
	if project.ParentProjectID != "" && !projectIDExists(projects, project.ParentProjectID) {
		return ProjectArchiveReceipt{}, errors.New("父项目不存在")
	}
	if err := validateSharedRepositoryRelation(projects, project, task.AllowedSourcePath); err != nil {
		return ProjectArchiveReceipt{}, err
	}
	for _, existing := range projects.Projects {
		if existing.ID == project.ID || strings.EqualFold(existing.Name, project.Name) {
			return ProjectArchiveReceipt{}, errors.New("同名项目已经登记")
		}
		if projectUsesSourcePath(existing, task.AllowedSourcePath) && projectRepositoryGroupID(existing) != projectRepositoryGroupID(project) {
			return ProjectArchiveReceipt{}, errors.New("该项目路径已经登记")
		}
	}
	if entryExisted {
		data, readErr := os.ReadFile(entryPath)
		if readErr != nil {
			return ProjectArchiveReceipt{}, readErr
		}
		const start = "<!-- brain2-project-identity:start -->"
		const end = "<!-- brain2-project-identity:end -->"
		if strings.Contains(string(data), start) && !strings.Contains(string(data), end) {
			return ProjectArchiveReceipt{}, errors.New("项目 README 中的 Brain2 身份区块不完整，未写入任何档案")
		}
	}
	if err := os.MkdirAll(filepath.Dir(entryPath), 0o755); err != nil {
		return ProjectArchiveReceipt{}, err
	}
	if !entryExisted {
		if err := atomicWrite(entryPath, []byte(renderArchivedProjectReadme(project)), 0o644); err != nil {
			return ProjectArchiveReceipt{}, err
		}
	}
	if err := writeProjectManifest(root, project); err != nil {
		if !entryExisted {
			_ = os.Remove(entryPath)
		}
		return ProjectArchiveReceipt{}, err
	}
	if entryExisted {
		if err := updateProjectIdentityBlock(entryPath, project); err != nil {
			return ProjectArchiveReceipt{}, err
		}
	}
	projects.SchemaVersion = ProjectRegistrySchemaVersion
	projects.Projects = append(projects.Projects, project)
	if err := writeJSON(filepath.Join(root, ".brain2", "projects.json"), projects); err != nil {
		return ProjectArchiveReceipt{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	candidates.Candidates[candidateIndex].Status = "archived"
	if err := writeJSON(filepath.Join(root, ".brain2", "project-candidates.json"), candidates); err != nil {
		return ProjectArchiveReceipt{}, err
	}
	task.Status = "completed"
	task.CompletedAt = now
	if err := writeJSON(projectArchiveTaskPath(root, task.TaskID), task); err != nil {
		return ProjectArchiveReceipt{}, err
	}
	receipt := ProjectArchiveReceipt{
		SchemaVersion: projectArchiveSchemaVersion, TaskID: task.TaskID, CandidateID: candidate.ID,
		ProjectID: project.ID, Status: "verified",
		ChangedPaths:      []string{project.Brain2Entry, project.ManifestPath, ".brain2/projects.json", ".brain2/project-candidates.json"},
		Evidence:          uniqueLimitedStrings(input.Evidence, 12, 240),
		NeedsVerification: uniqueLimitedStrings(input.NeedsVerification, 12, 240), CompletedAt: now,
	}
	if err := writeJSON(projectArchiveReceiptPath(root, task.TaskID), receipt); err != nil {
		return ProjectArchiveReceipt{}, err
	}
	return VerifyProjectArchiveReceipt(root, task.TaskID)
}

func VerifyProjectArchiveReceipt(root, taskID string) (ProjectArchiveReceipt, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ProjectArchiveReceipt{}, err
	}
	var receipt ProjectArchiveReceipt
	data, err := os.ReadFile(projectArchiveReceiptPath(root, taskID))
	if err != nil {
		return receipt, errors.New("项目建档回执不存在")
	}
	if err := json.Unmarshal(data, &receipt); err != nil {
		return receipt, errors.New("项目建档回执损坏")
	}
	task, err := LoadProjectArchiveTask(root, taskID)
	if err != nil || task.Status != "completed" || receipt.Status != "verified" {
		return receipt, errors.New("项目建档任务与回执状态不一致")
	}
	projects, err := LoadProjects(root)
	if err != nil {
		return receipt, err
	}
	for _, project := range projects.Projects {
		if project.ID != receipt.ProjectID {
			continue
		}
		manifest, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(project.ManifestPath)))
		if readErr != nil {
			return receipt, errors.New("项目资产登记文件不存在")
		}
		values := readManifestScalars(string(manifest))
		if values["project_id"] != project.ID || values["brain2_project_manifest"] != project.ManifestVersion {
			return receipt, errors.New("项目资产登记与机器索引不一致")
		}
		return receipt, nil
	}
	return receipt, errors.New("建档回执引用的项目不存在")
}

func validateProjectArchiveInput(root string, candidate ProjectCandidate, input ProjectArchiveInput) (Project, string, bool, error) {
	name, err := validateProjectName(input.Name)
	if err != nil {
		return Project{}, "", false, err
	}
	if strings.TrimSpace(input.Goal) == "" || len([]rune(input.Goal)) > 500 {
		return Project{}, "", false, errors.New("项目用途必须为 1-500 个字符")
	}
	if len(uniqueLimitedStrings(input.Evidence, 12, 240)) == 0 {
		return Project{}, "", false, errors.New("项目档案必须提供可验证依据")
	}
	if len(input.AdditionalPaths) > 12 {
		return Project{}, "", false, errors.New("additional_paths 不能超过 12 项")
	}
	entityKind := strings.TrimSpace(input.EntityKind)
	if !allowedValue(entityKind, "code_project", "application", "product", "needs_verification") {
		return Project{}, "", false, errors.New("entity_kind 必须是 code_project、application、product 或 needs_verification")
	}
	lifecycle := strings.TrimSpace(input.LifecycleStage)
	if !allowedValue(lifecycle, "discovery", "active", "paused", "completed", "archived", "needs_verification") {
		return Project{}, "", false, errors.New("lifecycle_stage 无效")
	}
	repositoryURL, err := normalizeGitHubRepository(input.RepositoryURL)
	if err != nil {
		return Project{}, "", false, err
	}
	defaultBranch := strings.TrimSpace(input.DefaultBranch)
	if len(defaultBranch) > 120 || strings.ContainsAny(defaultBranch, " \t\r\n") {
		return Project{}, "", false, errors.New("default_branch 无效")
	}
	id := safeID(name)
	sharedRepositoryProjectID, err := normalizeProjectReference(input.SharedRepositoryProjectID)
	if err != nil {
		return Project{}, "", false, err
	}
	candidateSharedRepositoryProjectID, err := normalizeProjectReference(candidate.SharedRepositoryProjectID)
	if err != nil {
		return Project{}, "", false, err
	}
	if sharedRepositoryProjectID == "" {
		sharedRepositoryProjectID = candidateSharedRepositoryProjectID
	} else if candidateSharedRepositoryProjectID != "" && sharedRepositoryProjectID != candidateSharedRepositoryProjectID {
		return Project{}, "", false, errors.New("共享仓库关系与用户确认的候选不一致")
	}
	parentProjectID := strings.TrimSpace(input.ParentProjectID)
	if parentProjectID != "" && sharedRepositoryProjectID != "" {
		return Project{}, "", false, errors.New("共享代码仓库不是父子项目关系，不能同时填写 parent_project_id")
	}
	if sharedRepositoryProjectID == id {
		return Project{}, "", false, errors.New("shared_repository_project_id 不能指向项目自身")
	}
	entry := filepath.ToSlash(strings.TrimSpace(input.Brain2Entry))
	defaultEntry := filepath.ToSlash(filepath.Join("02 项目", name, "README.md"))
	if entry == "" {
		entry = defaultEntry
	}
	entryPath, err := governedProjectEntryPath(root, entry)
	if err != nil || !strings.EqualFold(filepath.Ext(entryPath), ".md") {
		return Project{}, "", false, errors.New("brain2_entry 必须是 Brain2 内的 Markdown 项目入口")
	}
	entryExisted := false
	if info, statErr := os.Stat(entryPath); statErr == nil {
		if !info.Mode().IsRegular() {
			return Project{}, "", false, errors.New("brain2_entry 不是普通文件")
		}
		entryExisted = true
	} else if !os.IsNotExist(statErr) {
		return Project{}, "", false, statErr
	} else if !strings.EqualFold(entry, defaultEntry) {
		return Project{}, "", false, errors.New("新项目入口只能创建在默认 02 项目目录；其他位置必须已有入口文件")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sourcePath := filepath.Clean(candidate.RootPath)
	project := newManagedProject(root, Project{
		ID: id, Name: name, EntityKind: entityKind, ParentProjectID: parentProjectID, SharedRepositoryProjectID: sharedRepositoryProjectID,
		LifecycleStage: lifecycle, Mode: "external", SourcePath: &sourcePath,
		RepositoryURL: repositoryURL, DefaultBranch: defaultBranch, Brain2Entry: filepath.ToSlash(entry),
		Status: "active", Goal: strings.TrimSpace(input.Goal), Tags: []string{}, CreatedAt: now, UpdatedAt: now,
	}, sourcePath, "source_root", "absolute")
	for index, asset := range input.AdditionalPaths {
		path := strings.TrimSpace(asset.Path)
		if path == "" || len(path) > 1024 || strings.EqualFold(filepath.Clean(path), sourcePath) {
			continue
		}
		status := strings.TrimSpace(asset.Status)
		if !allowedValue(status, "verified", "missing", "stale", "unknown") {
			status = "unknown"
		}
		role := strings.TrimSpace(asset.Role)
		if role == "" {
			role = "device_worktree"
		}
		project.Assets = append(project.Assets, ProjectAsset{
			ID: fmt.Sprintf("device-path-%d", index+1), Role: role, Path: path, PathKind: "absolute",
			DeviceID: strings.TrimSpace(asset.DeviceID), Status: status, Truth: "current", Access: "metadata_only",
		})
	}
	return project, entryPath, entryExisted, nil
}

func normalizeGitHubRepository(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("repository_url 必须是无凭据的 GitHub HTTPS 仓库地址")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", errors.New("repository_url 必须指向具体 GitHub 仓库")
	}
	return "https://github.com/" + parts[0] + "/" + parts[1], nil
}

func renderArchivedProjectReadme(project Project) string {
	return fmt.Sprintf("---\nproject_id: %s\nproject_name: %s\nproject_status: active\nproject_mode: external\nsource: Agent via Meow Launcher\n---\n\n# %s\n\n## 项目目标\n\n%s\n", project.ID, project.Name, project.Name, project.Goal)
}

func projectIDExists(projects ProjectRegistry, projectID string) bool {
	for _, project := range projects.Projects {
		if project.ID == projectID {
			return true
		}
	}
	return false
}

func validateSharedRepositoryRelation(projects ProjectRegistry, project Project, sourcePath string) error {
	sharedProjectID := project.SharedRepositoryProjectID
	if sharedProjectID == "" {
		return nil
	}
	if !allowedValue(project.EntityKind, "application", "product") {
		return errors.New("共享代码仓库关系只适用于独立应用或产品")
	}
	for _, owner := range projects.Projects {
		if owner.ID != sharedProjectID {
			continue
		}
		if projectUsesSourcePath(owner, sourcePath) || sameRepositoryURL(owner.RepositoryURL, project.RepositoryURL) {
			return nil
		}
		return errors.New("共享仓库项目存在，但当前路径或 repository_url 与其代码仓库证据不一致")
	}
	return errors.New("shared_repository_project_id 引用的项目不存在")
}

func projectRepositoryGroupID(project Project) string {
	if project.SharedRepositoryProjectID != "" {
		return strings.ToLower(project.SharedRepositoryProjectID)
	}
	return strings.ToLower(project.ID)
}

func projectUsesSourcePath(project Project, sourcePath string) bool {
	sourcePath = filepath.Clean(strings.TrimSpace(sourcePath))
	if project.SourcePath != nil && strings.EqualFold(filepath.Clean(strings.TrimSpace(*project.SourcePath)), sourcePath) {
		return true
	}
	for _, asset := range project.Assets {
		if asset.PathKind == "absolute" && strings.EqualFold(filepath.Clean(strings.TrimSpace(asset.Path)), sourcePath) {
			return true
		}
	}
	return false
}

func sameRepositoryURL(left, right string) bool {
	return left != "" && right != "" && strings.EqualFold(strings.TrimSuffix(left, "/"), strings.TrimSuffix(right, "/"))
}

func allowedValue(value string, values ...string) bool {
	for _, allowed := range values {
		if value == allowed {
			return true
		}
	}
	return false
}

func projectArchiveTaskPath(root, taskID string) string {
	return filepath.Join(root, ".brain2", "runtime", "project-archives", taskID+".json")
}

func projectArchiveReceiptPath(root, taskID string) string {
	return filepath.Join(root, ".brain2", "project-archive-receipts", taskID+".json")
}
