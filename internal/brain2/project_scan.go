package brain2

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const projectCandidateSchemaVersion = "0.2"

type ProjectScanTask struct {
	SchemaVersion string `json:"schema_version"`
	TaskID        string `json:"task_id"`
	Nonce         string `json:"nonce"`
	AgentID       string `json:"agent_id"`
	WorkspaceID   string `json:"workspace_id"`
	DiscoveryMode string `json:"discovery_mode"`
	ScanRoot      string `json:"scan_root"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

type ProjectCandidateInput struct {
	Name                      string   `json:"name"`
	RootPath                  string   `json:"root_path"`
	SuggestedGoal             string   `json:"suggested_goal"`
	Confidence                float64  `json:"confidence"`
	Evidence                  []string `json:"evidence"`
	DetectedKind              string   `json:"detected_kind"`
	SuggestedConsumers        []string `json:"suggested_consumers"`
	SharedRepositoryProjectID string   `json:"shared_repository_project_id,omitempty"`
}

type ProjectCandidate struct {
	ID                        string   `json:"id"`
	ScanTaskID                string   `json:"scan_task_id"`
	AgentID                   string   `json:"agent_id"`
	Name                      string   `json:"name"`
	RootPath                  string   `json:"root_path"`
	SuggestedGoal             string   `json:"suggested_goal"`
	Confidence                float64  `json:"confidence"`
	Evidence                  []string `json:"evidence"`
	DetectedKind              string   `json:"detected_kind"`
	SuggestedConsumers        []string `json:"suggested_consumers"`
	ParentProjectID           string   `json:"parent_project_id,omitempty"`
	SharedRepositoryProjectID string   `json:"shared_repository_project_id,omitempty"`
	Status                    string   `json:"status"`
	SubmittedAt               string   `json:"submitted_at"`
}

type ProjectCandidateRegistry struct {
	SchemaVersion string             `json:"schema_version"`
	Candidates    []ProjectCandidate `json:"candidates"`
}

func CreateProjectScanTask(root, scanRoot, agentID string) (ProjectScanTask, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ProjectScanTask{}, err
	}
	discoveryMode := "brain2_first"
	scanRoot = strings.TrimSpace(scanRoot)
	if scanRoot != "" {
		scanRoot = filepath.Clean(scanRoot)
		info, statErr := os.Stat(scanRoot)
		if statErr != nil || !info.IsDir() {
			return ProjectScanTask{}, errors.New("扫描范围不存在或不是目录")
		}
		discoveryMode = "bounded_scan"
	}
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return ProjectScanTask{}, errors.New("请选择负责扫描的 Agent")
	}
	taskID, err := randomHex(12)
	if err != nil {
		return ProjectScanTask{}, err
	}
	nonce, err := randomHex(24)
	if err != nil {
		return ProjectScanTask{}, err
	}
	workspaceID, err := workspaceInstanceID(root)
	if err != nil {
		return ProjectScanTask{}, err
	}
	task := ProjectScanTask{
		SchemaVersion: projectCandidateSchemaVersion,
		TaskID:        taskID, Nonce: nonce, AgentID: agentID, WorkspaceID: workspaceID,
		DiscoveryMode: discoveryMode, ScanRoot: scanRoot,
		Status: "prepared", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	path := filepath.Join(root, ".brain2", "runtime", "project-scans", taskID+".json")
	if err := writeJSON(path, task); err != nil {
		return ProjectScanTask{}, err
	}
	return task, nil
}

func LoadProjectScanTask(root, taskID string) (ProjectScanTask, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ProjectScanTask{}, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || strings.ContainsAny(taskID, `/\\`) {
		return ProjectScanTask{}, errors.New("无效的项目扫描任务")
	}
	var task ProjectScanTask
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "runtime", "project-scans", taskID+".json"))
	if err != nil {
		return ProjectScanTask{}, errors.New("项目扫描任务不存在")
	}
	if err := json.Unmarshal(data, &task); err != nil {
		return ProjectScanTask{}, fmt.Errorf("项目扫描任务损坏: %w", err)
	}
	return task, nil
}

func LoadProjectCandidates(root string) (ProjectCandidateRegistry, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ProjectCandidateRegistry{}, err
	}
	registry := ProjectCandidateRegistry{SchemaVersion: projectCandidateSchemaVersion, Candidates: []ProjectCandidate{}}
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "project-candidates.json"))
	if os.IsNotExist(err) {
		return registry, nil
	}
	if err != nil {
		return registry, err
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return registry, fmt.Errorf("项目候选注册表损坏: %w", err)
	}
	if registry.Candidates == nil {
		registry.Candidates = []ProjectCandidate{}
	}
	return registry, nil
}

func SubmitProjectCandidates(root string, task ProjectScanTask, inputs []ProjectCandidateInput) (ProjectCandidateRegistry, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return ProjectCandidateRegistry{}, err
	}
	if task.Status != "prepared" && task.Status != "submitted" {
		return ProjectCandidateRegistry{}, errors.New("项目扫描任务当前不可提交")
	}
	registry, err := LoadProjectCandidates(root)
	if err != nil {
		return registry, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, input := range inputs {
		candidate, err := validateProjectCandidate(task, input, now)
		if err != nil {
			return registry, err
		}
		replaced := false
		for index := range registry.Candidates {
			if sameProjectCandidateIdentity(registry.Candidates[index], candidate) {
				candidate.ID = registry.Candidates[index].ID
				candidate.Status = registry.Candidates[index].Status
				if candidate.Status == "" {
					candidate.Status = "pending"
				}
				registry.Candidates[index] = candidate
				replaced = true
				break
			}
		}
		if !replaced {
			candidate.ID, err = randomHex(10)
			if err != nil {
				return registry, err
			}
			registry.Candidates = append(registry.Candidates, candidate)
		}
	}
	sort.Slice(registry.Candidates, func(i, j int) bool {
		if registry.Candidates[i].Status != registry.Candidates[j].Status {
			return registry.Candidates[i].Status < registry.Candidates[j].Status
		}
		return registry.Candidates[i].Confidence > registry.Candidates[j].Confidence
	})
	if err := writeJSON(filepath.Join(root, ".brain2", "project-candidates.json"), registry); err != nil {
		return registry, err
	}
	task.Status = "submitted"
	if err := writeJSON(filepath.Join(root, ".brain2", "runtime", "project-scans", task.TaskID+".json"), task); err != nil {
		return registry, err
	}
	return registry, nil
}

func validateProjectCandidate(task ProjectScanTask, input ProjectCandidateInput, now string) (ProjectCandidate, error) {
	rootPath := filepath.Clean(strings.TrimSpace(input.RootPath))
	if !filepath.IsAbs(rootPath) {
		return ProjectCandidate{}, errors.New("项目候选必须提供绝对路径")
	}
	if task.DiscoveryMode == "bounded_scan" || task.ScanRoot != "" {
		rel, relErr := filepath.Rel(task.ScanRoot, rootPath)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return ProjectCandidate{}, fmt.Errorf("项目候选超出用户授权扫描范围: %s", rootPath)
		}
	}
	info, err := os.Stat(rootPath)
	if err != nil || !info.IsDir() {
		return ProjectCandidate{}, fmt.Errorf("项目候选路径不存在或不是目录: %s", rootPath)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = filepath.Base(rootPath)
	}
	confidence := input.Confidence
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	evidence := uniqueLimitedStrings(input.Evidence, 8, 180)
	if len(evidence) == 0 {
		return ProjectCandidate{}, errors.New("项目候选必须提供可验证依据")
	}
	suggestedGoal := strings.TrimSpace(input.SuggestedGoal)
	if len([]rune(suggestedGoal)) > 300 {
		return ProjectCandidate{}, errors.New("项目用途候选不能超过 300 个字符")
	}
	detectedKind := strings.TrimSpace(input.DetectedKind)
	if !allowedProjectEntityKind(detectedKind) {
		return ProjectCandidate{}, errors.New("detected_kind 必须是 code_project、application、product 或 needs_verification")
	}
	sharedRepositoryProjectID, err := normalizeProjectReference(input.SharedRepositoryProjectID)
	if err != nil {
		return ProjectCandidate{}, err
	}
	return ProjectCandidate{
		ScanTaskID: task.TaskID, AgentID: task.AgentID, Name: name, RootPath: rootPath,
		SuggestedGoal: suggestedGoal, Confidence: confidence, Evidence: evidence, DetectedKind: detectedKind,
		SuggestedConsumers:        uniqueLimitedStrings(input.SuggestedConsumers, 4, 40),
		SharedRepositoryProjectID: sharedRepositoryProjectID,
		Status:                    "pending", SubmittedAt: now,
	}, nil
}

func sameProjectCandidateIdentity(existing, candidate ProjectCandidate) bool {
	if !strings.EqualFold(filepath.Clean(existing.RootPath), filepath.Clean(candidate.RootPath)) {
		return false
	}
	if existing.SharedRepositoryProjectID == "" && candidate.SharedRepositoryProjectID == "" {
		return true
	}
	return strings.EqualFold(existing.Name, candidate.Name) &&
		strings.EqualFold(existing.SharedRepositoryProjectID, candidate.SharedRepositoryProjectID)
}

func normalizeProjectReference(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	if len(value) > 120 || safeID(value) != value {
		return "", errors.New("shared_repository_project_id 必须是已有项目的规范 project_id")
	}
	return value, nil
}

func uniqueLimitedStrings(values []string, limit, maxLength int) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > maxLength || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func ConfirmProjectCandidate(root, candidateID string) error {
	return SelectProjectCandidateForArchive(root, candidateID)
}

func SelectProjectCandidateForArchive(root, candidateID string) error {
	root, err := requireWorkspace(root)
	if err != nil {
		return err
	}
	registry, err := LoadProjectCandidates(root)
	if err != nil {
		return err
	}
	for index := range registry.Candidates {
		candidate := &registry.Candidates[index]
		if candidate.ID != candidateID {
			continue
		}
		if candidate.Status == "selected_for_archiving" || candidate.Status == "archived" {
			return nil
		}
		if candidate.Status != "pending" {
			return errors.New("项目候选当前不能加入建档")
		}
		if !allowedProjectEntityKind(candidate.DetectedKind) {
			return errors.New("该旧候选不属于代码、应用或产品项目；请忽略或归入已有项目")
		}
		candidate.Status = "selected_for_archiving"
		candidate.ParentProjectID = ""
		return writeJSON(filepath.Join(root, ".brain2", "project-candidates.json"), registry)
	}
	return errors.New("项目候选不存在")
}

func allowedProjectEntityKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "code_project", "application", "product", "needs_verification":
		return true
	default:
		return false
	}
}

func AssignProjectCandidateToParent(root, candidateID, parentProjectID string) error {
	root, err := requireWorkspace(root)
	if err != nil {
		return err
	}
	projects, err := LoadProjects(root)
	if err != nil {
		return err
	}
	parentProjectID = strings.TrimSpace(parentProjectID)
	parentExists := false
	for _, project := range projects.Projects {
		if project.ID == parentProjectID && project.Status == "active" {
			parentExists = true
			break
		}
	}
	if !parentExists {
		return errors.New("归属的已有项目不存在或不可用")
	}
	registry, err := LoadProjectCandidates(root)
	if err != nil {
		return err
	}
	for index := range registry.Candidates {
		candidate := &registry.Candidates[index]
		if candidate.ID != candidateID {
			continue
		}
		if candidate.Status != "pending" && candidate.Status != "selected_for_archiving" {
			return errors.New("项目候选当前不能调整归属")
		}
		candidate.Status = "assigned_to_parent"
		candidate.ParentProjectID = parentProjectID
		return writeJSON(filepath.Join(root, ".brain2", "project-candidates.json"), registry)
	}
	return errors.New("项目候选不存在")
}

func registerProjectCandidate(root string, candidate ProjectCandidate) error {
	name, err := validateProjectName(candidate.Name)
	if err != nil {
		return err
	}
	sourcePath, err := normalizeRoot(candidate.RootPath)
	if err != nil {
		return errors.New("项目候选缺少有效的绝对路径")
	}
	info, err := os.Stat(sourcePath)
	if err != nil || !info.IsDir() {
		return errors.New("项目候选路径已不存在或不是目录")
	}
	projects, err := LoadProjects(root)
	if err != nil {
		return err
	}
	id := safeID(name)
	for _, project := range projects.Projects {
		if project.SourcePath != nil && strings.EqualFold(filepath.Clean(*project.SourcePath), sourcePath) {
			return nil
		}
		if strings.EqualFold(project.Name, name) {
			return errors.New("同名项目已登记；请先确认是否为同一项目")
		}
		if project.ID == id {
			return errors.New("项目 ID 与已有登记冲突；请先调整候选名称")
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	project := newMachineRegisteredProject(root, Project{
		ID: id, Name: name, Mode: "external", SourcePath: &sourcePath,
		Status: "active", Goal: strings.TrimSpace(candidate.SuggestedGoal),
		Tags: []string{}, CreatedAt: now, UpdatedAt: now,
	}, sourcePath)
	if err := writeMachineProjectRecord(root, project, candidate); err != nil {
		return err
	}
	projects.SchemaVersion = ProjectRegistrySchemaVersion
	projects.Projects = append(projects.Projects, project)
	if err := writeJSON(filepath.Join(root, ".brain2", "projects.json"), projects); err != nil {
		_ = os.Remove(filepath.Join(root, filepath.FromSlash(project.Brain2Entry)))
		return err
	}
	return nil
}

func RegisterExternalProjectMetadata(root, name, sourcePath, goal string) error {
	return registerProjectCandidate(root, ProjectCandidate{
		Name: name, RootPath: sourcePath, SuggestedGoal: goal,
		Confidence: 1, Evidence: []string{"用户在 Launcher 中手动选择并确认该目录"},
		DetectedKind: "manual_registration", Status: "confirmed",
		SubmittedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func IgnoreProjectCandidate(root, candidateID string) error {
	registry, err := LoadProjectCandidates(root)
	if err != nil {
		return err
	}
	for index := range registry.Candidates {
		if registry.Candidates[index].ID == candidateID {
			registry.Candidates[index].Status = "ignored"
			return writeJSON(filepath.Join(root, ".brain2", "project-candidates.json"), registry)
		}
	}
	return errors.New("项目候选不存在")
}
