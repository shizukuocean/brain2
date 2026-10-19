package brain2

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const schemaVersion = "0.1"

type Config struct {
	SchemaVersion  string   `json:"schema_version"`
	InstanceID     string   `json:"instance_id,omitempty"`
	Brain2Root     string   `json:"brain2_root"`
	Initialized    bool     `json:"initialized"`
	DeviceName     string   `json:"device_name"`
	Language       string   `json:"language"`
	CreatedAt      string   `json:"created_at"`
	LastOpenedAt   string   `json:"last_opened_at"`
	BootstrapFiles []string `json:"bootstrap_files,omitempty"`
	Launcher       struct {
		Product string `json:"product"`
		Version string `json:"version"`
	} `json:"launcher"`
}

func LoadConfig(root string) (Config, error) {
	var config Config
	data, err := os.ReadFile(filepath.Join(filepath.Clean(root), ".brain2", "config.json"))
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	if strings.TrimSpace(config.InstanceID) == "" || !config.Initialized {
		return Config{}, errors.New("Brain2 配置缺少有效的逻辑大脑身份")
	}
	return config, nil
}

func legacyBootstrapFiles() []string {
	return []string{
		"NAV.md",
		filepath.ToSlash(filepath.Join("06 AI助理协同", "START_HERE.md")),
		filepath.ToSlash(filepath.Join("06 AI助理协同", "规则", "大脑治理总则.md")),
		filepath.ToSlash(filepath.Join("06 AI助理协同", "当前上下文.md")),
	}
}

func managedBootstrapFiles() []string {
	return []string{
		".brain2/framework/NAV.md",
		".brain2/framework/START_HERE.md",
		".brain2/framework/GOVERNANCE.md",
		".brain2/framework/CURRENT_CONTEXT.md",
	}
}

func workspaceBootstrapFiles(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "config.json"))
	if err == nil {
		var cfg Config
		if json.Unmarshal(data, &cfg) == nil && len(cfg.BootstrapFiles) > 0 {
			return append([]string{}, cfg.BootstrapFiles...)
		}
	}
	return legacyBootstrapFiles()
}

type Project struct {
	ID                        string         `json:"id"`
	Name                      string         `json:"name"`
	EntityKind                string         `json:"entity_kind,omitempty"`
	ParentProjectID           string         `json:"parent_project_id,omitempty"`
	SharedRepositoryProjectID string         `json:"shared_repository_project_id,omitempty"`
	LifecycleStage            string         `json:"lifecycle_stage,omitempty"`
	Mode                      string         `json:"mode"`
	SourcePath                *string        `json:"source_path"`
	RepositoryURL             string         `json:"repository_url,omitempty"`
	DefaultBranch             string         `json:"default_branch,omitempty"`
	Brain2Entry               string         `json:"brain2_entry"`
	Status                    string         `json:"status"`
	Goal                      string         `json:"goal"`
	Tags                      []string       `json:"tags"`
	ManifestPath              string         `json:"manifest_path,omitempty"`
	ManifestVersion           string         `json:"manifest_version,omitempty"`
	AssetStatus               string         `json:"asset_status,omitempty"`
	PathStatus                string         `json:"path_status,omitempty"`
	TruthStatus               string         `json:"truth_status,omitempty"`
	ConnectionGate            string         `json:"connection_gate,omitempty"`
	Capabilities              []string       `json:"capabilities,omitempty"`
	Assets                    []ProjectAsset `json:"assets,omitempty"`
	CreatedAt                 string         `json:"created_at"`
	UpdatedAt                 string         `json:"updated_at"`
}

type ProjectRegistry struct {
	SchemaVersion string    `json:"schema_version"`
	Projects      []Project `json:"projects"`
}

func (project Project) SourcePathValue() string {
	if project.SourcePath == nil {
		return "未登记"
	}
	return *project.SourcePath
}

type Agent struct {
	DeviceID         string `json:"device_id,omitempty"`
	ID               string `json:"id"`
	Name             string `json:"name"`
	Surface          string `json:"surface,omitempty"`
	SupportLevel     string `json:"support_level"`
	AdapterLoaded    bool   `json:"adapter_loaded"`
	ConnectionStatus string `json:"connection_status,omitempty"`
	ConnectionMode   string `json:"connection_mode,omitempty"`
	ConnectionID     string `json:"connection_id,omitempty"`
	RuntimeVersion   string `json:"runtime_version"`
	AdapterVersion   string `json:"adapter_version"`
	ConnectedAt      string `json:"connected_at,omitempty"`
	LastError        string `json:"last_error,omitempty"`
	UpdatedAt        string `json:"updated_at"`
}

type AgentRegistry struct {
	SchemaVersion string  `json:"schema_version"`
	Agents        []Agent `json:"agents"`
}

func normalizeRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("Brain2 目录不能为空")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("解析 Brain2 目录失败: %w", err)
	}
	return filepath.Clean(absRoot), nil
}

func IsWorkspace(root string) bool {
	root, err := normalizeRoot(root)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "config.json"))
	if err != nil {
		return false
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil || !cfg.Initialized || cfg.SchemaVersion == "" {
		return false
	}
	// brain2_root/device_name are legacy device-local hints. They are synced with
	// the logical Brain2 and therefore cannot identify the machine opening it.
	// The shared instance_id identifies the logical brain; devices.json and the
	// Launcher's local settings identify individual devices.
	return strings.TrimSpace(cfg.InstanceID) != ""
}

func workspaceRootIdentity(root string) (string, bool) {
	value := strings.TrimSpace(strings.ReplaceAll(root, `\`, "/"))
	value = strings.TrimRight(value, "/")
	if len(value) >= 3 && value[1] == ':' && value[2] == '/' {
		drive := strings.ToLower(value[:1])
		rest := strings.TrimLeft(value[3:], "/")
		return drive + ":/" + strings.ToLower(rest), true
	}
	lower := strings.ToLower(value)
	if len(lower) >= 7 && strings.HasPrefix(lower, "/mnt/") && lower[6] == '/' {
		drive := lower[5:6]
		rest := strings.TrimLeft(lower[7:], "/")
		return drive + ":/" + rest, true
	}
	if strings.HasPrefix(value, "/") {
		return strings.ToLower(filepath.ToSlash(filepath.Clean(value))), true
	}
	if filepath.IsAbs(root) {
		return strings.ToLower(filepath.ToSlash(filepath.Clean(root))), true
	}
	return "", false
}

func IsExistingBrain2(root string) bool {
	root, err := normalizeRoot(root)
	if err != nil {
		return false
	}
	required := []string{
		"NAV.md",
		filepath.Join("06 AI助理协同", "START_HERE.md"),
		filepath.Join("06 AI助理协同", "规则", "大脑治理总则.md"),
	}
	for _, rel := range required {
		info, statErr := os.Stat(filepath.Join(root, rel))
		if statErr != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

func AttachExisting(root string) error {
	root, err := normalizeRoot(root)
	if err != nil {
		return err
	}
	if IsWorkspace(root) {
		return nil
	}
	info, statErr := os.Stat(root)
	if statErr != nil || !info.IsDir() {
		return errors.New("所选 Obsidian 仓库不存在或不是目录")
	}
	legacy := IsExistingBrain2(root)
	metadataDir := filepath.Join(root, ".brain2")
	if info, statErr := os.Stat(filepath.Join(metadataDir, "config.json")); statErr == nil && info.Mode().IsRegular() {
		return errors.New("检测到已有但无法识别的 .brain2/config.json；为保护数据，未自动覆盖")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	for _, rel := range []string{"runtime", "backups", "logs", "connections"} {
		if err := os.MkdirAll(filepath.Join(metadataDir, rel), 0o755); err != nil {
			return err
		}
	}
	bootstrapFiles := legacyBootstrapFiles()
	needsFramework := !legacy
	if legacy {
		available := []string{}
		for _, rel := range legacyBootstrapFiles() {
			if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil && info.Mode().IsRegular() {
				available = append(available, rel)
			} else {
				needsFramework = true
			}
		}
		if needsFramework {
			bootstrapFiles = append(available, managedBootstrapFiles()...)
		}
	}
	if needsFramework {
		if !legacy {
			bootstrapFiles = managedBootstrapFiles()
		}
		frameworkFiles := map[string]string{
			"NAV.md":             "# Brain2 连接导航\n\n这是 Launcher 管理的增量连接层。原 Obsidian 内容保持原位，不会被自动分类或迁移。\n",
			"START_HERE.md":      "# Agent 入口\n\n1. 先核对 workspace ID 与实际根目录。\n2. 旧内容默认只读。\n3. 语义整理、分类和修复必须先向用户提交方案并获得授权。\n",
			"GOVERNANCE.md":      "# Brain2 治理边界\n\n- Launcher 只管理连接、机器索引和确定性证据。\n- Agent 负责理解与判断；不得把推测直接写成用户事实。\n- 不移动、不重命名、不删除、不静默改写既有内容。\n- 项目发现不等于 Blueprint 或 Protector 已获授权。\n",
			"CURRENT_CONTEXT.md": "# 当前上下文\n\n尚未由用户与 Agent 建立当前工作上下文。\n",
		}
		for name, content := range frameworkFiles {
			if err := atomicWrite(filepath.Join(metadataDir, "framework", name), []byte(content), 0o644); err != nil {
				return err
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	device, _ := os.Hostname()
	instanceID, err := randomHex(12)
	if err != nil {
		return err
	}
	cfg := Config{SchemaVersion: schemaVersion, InstanceID: "brain2-" + instanceID, Brain2Root: root, Initialized: true, DeviceName: device, Language: "zh-CN", CreatedAt: now, LastOpenedAt: now, BootstrapFiles: bootstrapFiles}
	cfg.Launcher.Product = "Meow Launcher"
	cfg.Launcher.Version = "0.6.6"
	if err := writeJSON(filepath.Join(metadataDir, "config.json"), cfg); err != nil {
		return err
	}
	if err := writeJSONIfMissing(filepath.Join(metadataDir, "projects.json"), ProjectRegistry{SchemaVersion: ProjectRegistrySchemaVersion, Projects: []Project{}}); err != nil {
		return err
	}
	if err := writeJSONIfMissing(filepath.Join(metadataDir, "agents.json"), AgentRegistry{SchemaVersion: schemaVersion, Agents: []Agent{}}); err != nil {
		return err
	}
	return writeJSONIfMissing(filepath.Join(metadataDir, "version.json"), map[string]any{
		"schema_version": schemaVersion, "brain2_product": "existing-vault", "launcher": "0.6.6",
		"protocol": ConnectionProtocolVersion, "attached_at": now, "migration_required": false,
	})
}

func requireWorkspace(root string) (string, error) {
	root, err := normalizeRoot(root)
	if err != nil {
		return "", err
	}
	if !IsWorkspace(root) {
		return "", errors.New("所选目录不是有效的 Brain2；请先安装或接入 Brain2")
	}
	return root, nil
}

func Initialize(root string) error {
	root, err := normalizeRoot(root)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(root); statErr == nil {
		if !info.IsDir() {
			return errors.New("目标路径不是文件夹")
		}
		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			return fmt.Errorf("读取目标目录失败: %w", readErr)
		}
		if len(entries) > 0 {
			return errors.New("目标目录不是空目录；为保护数据，安装已停止")
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("检查目标目录失败: %w", statErr)
	} else if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	dirs := []string{
		"00 收件箱", "01 关于我", "02 项目", "03 资料库",
		filepath.Join("06 AI助理协同", "规则"),
		filepath.Join("06 AI助理协同", "任务"),
		filepath.Join("06 AI助理协同", "事件"),
		filepath.Join("06 AI助理协同", "agent工作区"),
		"10 项目沉淀", "99 归档",
		filepath.Join(".brain2", "runtime"),
		filepath.Join(".brain2", "backups"),
		filepath.Join(".brain2", "logs"),
	}
	for _, rel := range dirs {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", rel, err)
		}
	}

	files := map[string]string{
		"NAV.md":              "# Brain2 导航\n\n- [[AGENT_START_HERE]]\n- [[06 AI助理协同/START_HERE]]\n- [[02 项目/README]]\n",
		"AGENT_START_HERE.md": "# AGENT START HERE\n\n1. 读取 NAV.md。\n2. 读取 06 AI助理协同/规则/大脑治理总则.md。\n3. 读取 06 AI助理协同/当前上下文.md。\n4. 读取当前项目 README。\n5. 写入前确认边界。\n",
		filepath.Join("00 收件箱", "README.md"):                "# 收件箱\n",
		filepath.Join("01 关于我", "README.md"):                "# 关于我\n",
		filepath.Join("02 项目", "README.md"):                 "# 项目\n",
		filepath.Join("03 资料库", "README.md"):                "# 资料库\n",
		filepath.Join("06 AI助理协同", "START_HERE.md"):         "# AI 助理协同\n",
		filepath.Join("06 AI助理协同", "当前上下文.md"):              "# 当前上下文\n\n- 当前重点：尚未填写\n",
		filepath.Join("06 AI助理协同", "管家视图.md"):               "# 管家视图\n",
		filepath.Join("06 AI助理协同", "规则", "大脑治理总则.md"):       "# 大脑治理总则\n\n- Brain2 是治理真值。\n- 本地项目文件、数据和代码是证据真值。\n- 先读后写。\n- 不确定时明确标注读取缺口。\n",
		filepath.Join("06 AI助理协同", "任务", "README.md"):       "# 任务\n",
		filepath.Join("06 AI助理协同", "事件", "README.md"):       "# 事件\n",
		filepath.Join("06 AI助理协同", "agent工作区", "README.md"): "# Agent 工作区\n",
		filepath.Join("10 项目沉淀", "README.md"):               "# 项目沉淀\n",
		filepath.Join("99 归档", "README.md"):                 "# 归档\n",
	}
	for rel, content := range files {
		if err := atomicWrite(filepath.Join(root, rel), []byte(content), 0o644); err != nil {
			return err
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	device, _ := os.Hostname()
	instanceID, err := randomHex(12)
	if err != nil {
		return err
	}
	cfg := Config{SchemaVersion: schemaVersion, InstanceID: "brain2-" + instanceID, Brain2Root: root, Initialized: true, DeviceName: device, Language: "zh-CN", CreatedAt: now, LastOpenedAt: now, BootstrapFiles: legacyBootstrapFiles()}
	cfg.Launcher.Product = "Meow Launcher"
	cfg.Launcher.Version = "0.6.6"
	if err := writeJSON(filepath.Join(root, ".brain2", "config.json"), cfg); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(root, ".brain2", "projects.json"), ProjectRegistry{SchemaVersion: ProjectRegistrySchemaVersion, Projects: []Project{}}); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(root, ".brain2", "agents.json"), AgentRegistry{SchemaVersion: schemaVersion, Agents: []Agent{}}); err != nil {
		return err
	}
	return writeJSON(filepath.Join(root, ".brain2", "version.json"), map[string]any{
		"schema_version": schemaVersion, "brain2_product": "0.3.0", "launcher": "0.6.6",
		"protocol": "0.1", "installed_at": now, "migration_required": false,
	})
}

func validateProjectName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("项目名称不能为空")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `\/:*?"<>|`) {
		return "", errors.New("项目名称包含 Windows 不允许的字符")
	}
	return name, nil
}

func hasProject(reg ProjectRegistry, id, name string) bool {
	for _, project := range reg.Projects {
		if project.ID == id || strings.EqualFold(project.Name, name) {
			return true
		}
	}
	return false
}

func CreateInternalProject(root, name, goal string) error {
	root, err := requireWorkspace(root)
	if err != nil {
		return err
	}
	name, err = validateProjectName(name)
	if err != nil {
		return err
	}
	reg, err := LoadProjects(root)
	if err != nil {
		return err
	}
	id := safeID(name)
	projectDir := filepath.Join(root, "02 项目", name)
	if hasProject(reg, id, name) {
		return errors.New("同名项目已经登记")
	}
	if _, err := os.Stat(projectDir); err == nil {
		return errors.New("同名项目目录已经存在")
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, rel := range []string{"知识库", "进度", "产出", "_暂存"} {
		if err := os.MkdirAll(filepath.Join(projectDir, rel), 0o755); err != nil {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	entry := filepath.ToSlash(filepath.Join("02 项目", name, "README.md"))
	manifestPath := filepath.ToSlash(filepath.Join("02 项目", name, projectManifestFilename))
	project := newManagedProject(root, Project{
		ID: id, Name: name, Mode: "internal", Brain2Entry: entry, Status: "active",
		Goal: strings.TrimSpace(goal), Tags: []string{}, CreatedAt: now, UpdatedAt: now,
	}, filepath.ToSlash(filepath.Join("02 项目", name)), "brain2_project_root", "brain2_relative")
	readme := fmt.Sprintf("---\nproject_id: %s\nproject_name: %s\nproject_status: active\nproject_mode: internal\ncreated_at: %s\nsource: Meow Launcher\n---\n\n# %s\n\n> 项目身份证：[[%s|项目资产登记]]\n\n## 项目目标\n\n%s\n\n## 当前读取缺口\n\n- 尚未读取项目材料\n", id, name, now, name, strings.TrimSuffix(manifestPath, ".md"), strings.TrimSpace(goal))
	if err := atomicWrite(filepath.Join(projectDir, "README.md"), []byte(readme), 0o644); err != nil {
		return err
	}
	if err := writeProjectManifest(root, project); err != nil {
		return err
	}
	reg.SchemaVersion = ProjectRegistrySchemaVersion
	reg.Projects = append(reg.Projects, project)
	return writeJSON(filepath.Join(root, ".brain2", "projects.json"), reg)
}

func AttachExternalProject(root, name, sourcePath, goal string) error {
	root, err := requireWorkspace(root)
	if err != nil {
		return err
	}
	name, err = validateProjectName(name)
	if err != nil {
		return err
	}
	sourcePath, err = normalizeRoot(sourcePath)
	if err != nil {
		return errors.New("外部项目目录不能为空")
	}
	info, err := os.Stat(sourcePath)
	if err != nil || !info.IsDir() {
		return errors.New("外部项目目录不存在")
	}
	reg, err := LoadProjects(root)
	if err != nil {
		return err
	}
	id := safeID(name)
	entryDir := filepath.Join(root, "02 项目", name)
	if hasProject(reg, id, name) {
		return errors.New("同名项目已经登记")
	}
	if _, err := os.Stat(entryDir); err == nil {
		return errors.New("同名项目目录已经存在")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	entry := filepath.ToSlash(filepath.Join("02 项目", name, "README.md"))
	manifestPath := filepath.ToSlash(filepath.Join("02 项目", name, projectManifestFilename))
	sp := sourcePath
	project := newManagedProject(root, Project{
		ID: id, Name: name, Mode: "external", SourcePath: &sp, Brain2Entry: entry,
		Status: "active", Goal: strings.TrimSpace(goal), Tags: []string{}, CreatedAt: now, UpdatedAt: now,
	}, sourcePath, "source_root", "absolute")
	readme := fmt.Sprintf("---\nproject_id: %s\nproject_name: %s\nproject_status: active\nproject_mode: external\nsource_path: %q\ncreated_at: %s\nsource: Meow Launcher\n---\n\n# %s\n\n> 项目身份证：[[%s|项目资产登记]]\n\n## 项目目标\n\n%s\n\n## 原始文件位置\n\n`%s`\n\nBrain2 不复制、不移动该目录。\n", id, name, sourcePath, now, name, strings.TrimSuffix(manifestPath, ".md"), strings.TrimSpace(goal), sourcePath)
	if err := atomicWrite(filepath.Join(entryDir, "README.md"), []byte(readme), 0o644); err != nil {
		return err
	}
	if err := writeProjectManifest(root, project); err != nil {
		return err
	}
	reg.SchemaVersion = ProjectRegistrySchemaVersion
	reg.Projects = append(reg.Projects, project)
	return writeJSON(filepath.Join(root, ".brain2", "projects.json"), reg)
}

// ConfirmProjectManifest upgrades one legacy project after an explicit user
// confirmation, or safely relocates an already managed external project. It
// never guesses a missing external path and never moves or copies source data.
func ConfirmProjectManifest(root, projectID, confirmedSourcePath string) error {
	return ConfirmProjectManifestForDevice(root, projectID, confirmedSourcePath, "")
}

// ConfirmProjectManifestForDevice records a verified external project path for
// one logical device without replacing paths owned by other devices.
func ConfirmProjectManifestForDevice(root, projectID, confirmedSourcePath, deviceID string) error {
	root, err := requireWorkspace(root)
	if err != nil {
		return err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return errors.New("项目 ID 不能为空")
	}
	reg, err := LoadProjects(root)
	if err != nil {
		return err
	}
	index := -1
	for i := range reg.Projects {
		if reg.Projects[i].ID == projectID {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("Brain2 中没有项目 %q", projectID)
	}
	project := reg.Projects[index]
	entryPath, err := governedProjectEntryPath(root, project.Brain2Entry)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(entryPath); statErr != nil || info.IsDir() {
		return errors.New("项目入口 README 不存在，无法确认身份")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	project.UpdatedAt = now
	switch project.Mode {
	case "internal":
		assetPath := filepath.ToSlash(filepath.Dir(project.Brain2Entry))
		assetFull, pathErr := governedProjectEntryPath(root, filepath.ToSlash(filepath.Join(assetPath, "README.md")))
		if pathErr != nil {
			return pathErr
		}
		if info, statErr := os.Stat(filepath.Dir(assetFull)); statErr != nil || !info.IsDir() {
			return errors.New("内部项目目录不存在，无法确认路径")
		}
		project = newManagedProject(root, project, assetPath, "brain2_project_root", "brain2_relative")
	case "external":
		sourcePath := strings.TrimSpace(confirmedSourcePath)
		if sourcePath == "" && project.SourcePath != nil {
			sourcePath = strings.TrimSpace(*project.SourcePath)
		}
		sourcePath, err = normalizeRoot(sourcePath)
		if err != nil {
			return errors.New("请由用户选择并确认外部项目目录")
		}
		if info, statErr := os.Stat(sourcePath); statErr != nil || !info.IsDir() {
			return errors.New("用户确认的外部项目目录不存在")
		}
		deviceID = strings.ToUpper(strings.TrimSpace(deviceID))
		if deviceID == "" {
			project.SourcePath = &sourcePath
			project = newManagedProject(root, project, sourcePath, "source_root", "absolute")
		} else {
			if _, normalizeErr := NormalizeDeviceID(deviceID); normalizeErr != nil {
				return normalizeErr
			}
			if project.SourcePath == nil || InferLegacyDeviceID(*project.SourcePath, "") == deviceID {
				project.SourcePath = &sourcePath
			}
			project = upsertManagedDeviceAsset(project, sourcePath, "source_root", "absolute", deviceID)
		}
	default:
		return fmt.Errorf("项目 %s 的模式 %q 不受支持", project.ID, project.Mode)
	}
	if err := writeProjectManifest(root, project); err != nil {
		return err
	}
	if err := updateProjectIdentityBlock(entryPath, project); err != nil {
		return err
	}
	reg.SchemaVersion = ProjectRegistrySchemaVersion
	reg.Projects[index] = project
	return writeJSON(filepath.Join(root, ".brain2", "projects.json"), reg)
}

func governedProjectEntryPath(root, entry string) (string, error) {
	entry = filepath.Clean(filepath.FromSlash(strings.TrimSpace(entry)))
	if entry == "." || filepath.IsAbs(entry) {
		return "", errors.New("项目入口必须是 Brain2 内的相对路径")
	}
	target := filepath.Join(root, entry)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("项目入口超出 Brain2 根目录")
	}
	return target, nil
}

func updateProjectIdentityBlock(entryPath string, project Project) error {
	data, err := os.ReadFile(entryPath)
	if err != nil {
		return err
	}
	const start = "<!-- brain2-project-identity:start -->"
	const end = "<!-- brain2-project-identity:end -->"
	var block strings.Builder
	fmt.Fprintf(&block, "%s\n## Brain2 项目身份\n\n", start)
	fmt.Fprintf(&block, "- 项目身份证：[[%s|项目资产登记]]\n", strings.TrimSuffix(project.ManifestPath, ".md"))
	if project.Mode == "external" && project.SourcePath != nil {
		fmt.Fprintf(&block, "- 当前源路径：`%s`\n", *project.SourcePath)
	}
	fmt.Fprintf(&block, "%s", end)
	content := string(data)
	if startIndex := strings.Index(content, start); startIndex >= 0 {
		if endOffset := strings.Index(content[startIndex:], end); endOffset >= 0 {
			endIndex := startIndex + endOffset + len(end)
			content = content[:startIndex] + block.String() + content[endIndex:]
		} else {
			return errors.New("项目 README 中的 Brain2 身份区块不完整，未自动覆盖")
		}
	} else {
		content = strings.TrimRight(content, "\r\n") + "\n\n" + block.String() + "\n"
	}
	return atomicWrite(entryPath, []byte(content), 0o644)
}

func LoadProjects(root string) (ProjectRegistry, error) {
	var reg ProjectRegistry
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "projects.json"))
	if err != nil {
		return reg, err
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return reg, err
	}
	if reg.Projects == nil {
		reg.Projects = []Project{}
	}
	return reg, nil
}

func EnsureInstanceID(root string) (Config, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return Config{}, err
	}
	path := filepath.Join(root, ".brain2", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(cfg.InstanceID) != "" {
		return cfg, nil
	}
	id, err := randomHex(12)
	if err != nil {
		return Config{}, err
	}
	cfg.InstanceID = "brain2-" + id
	if err := writeJSON(path, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadAgents(root string) (AgentRegistry, error) {
	var reg AgentRegistry
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "agents.json"))
	if err != nil {
		return reg, err
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return reg, err
	}
	if reg.Agents == nil {
		reg.Agents = []Agent{}
	}
	return reg, nil
}

func PrepareRuntime(root, agentID string) error {
	return PrepareRuntimeForDevice(root, "A", agentID)
}

func PrepareRuntimeForDevice(root, deviceID, agentID string) error {
	root, err := requireWorkspace(root)
	if err != nil {
		return err
	}
	deviceID, err = NormalizeDeviceID(deviceID)
	if err != nil {
		return err
	}
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		agentID = "codex"
	}
	definition, ok := AgentDefinitionByID(agentID)
	if !ok {
		return fmt.Errorf("当前预览版尚未支持 Agent %q", agentID)
	}
	dir := filepath.Join(root, ".brain2", "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	files := map[string]string{
		"AGENT_START_HERE.md": fmt.Sprintf("# Brain2 Runtime Entry\n\nBrain2 根目录：`%s`\n\n读取顺序：\n\n1. `AGENT_START_HERE.md`\n2. `06 AI助理协同/规则/大脑治理总则.md`\n3. `06 AI助理协同/当前上下文.md`\n4. `.brain2/projects.json`\n5. 当前项目 README\n\n写入前先确认任务归属、文件归属和下游链接。\n", root),
		"FIRST_HANDSHAKE.md":  "# First Handshake\n\n请确认：\n\n- 已读取 Brain2 固定入口与治理规则；\n- 已识别当前设备、Agent 和任务；\n- 将代码保存在本地开发仓库，不把云盘目录当作实时开发根；\n- 写入 Brain2 后会补齐任务记录与项目进度。\n",
		"active-protocol.md":  "# Active Protocol\n\n- Protocol: Brain2 Shared Brain Governance\n- Version: 0.1\n- Mode: local-first, evidence-linked\n",
		"current-adapter.md":  fmt.Sprintf("# Current Adapter\n\n- Device: %s\n- Agent: %s (%s)\n- Adapter: %s-local\n- Surface: %s\n- Version: 0.3.0\n- Support: %s\n", deviceID, definition.Name, deviceID, definition.ID, definition.Surface, definition.SupportLevel),
	}
	for name, content := range files {
		if err := atomicWrite(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	if err := writeJSON(filepath.Join(dir, "agent-config.json"), map[string]any{
		"schema_version": schemaVersion, "device_id": deviceID, "agent_id": definition.ID, "brain2_root": root,
		"entrypoint": "AGENT_START_HERE.md", "handshake": "FIRST_HANDSHAKE.md",
	}); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "runtime-version.json"), map[string]any{
		"schema_version": schemaVersion, "runtime_version": "0.3.0", "adapter_version": "0.3.0", "generated_at": now,
	}); err != nil {
		return err
	}
	reg, err := LoadAgents(root)
	if err != nil {
		return err
	}
	agent := Agent{DeviceID: deviceID, ID: definition.ID, Name: definition.Name, Surface: definition.Surface, SupportLevel: definition.SupportLevel, AdapterLoaded: false, ConnectionStatus: string(ConnectionPrepared), RuntimeVersion: "0.3.0", AdapterVersion: "0.3.0", UpdatedAt: now}
	replaced := false
	for i := range reg.Agents {
		if reg.Agents[i].DeviceID == deviceID && reg.Agents[i].ID == agent.ID {
			// Runtime files may be regenerated when a Prompt is prepared. Preserve a
			// previously installed/connected adapter instead of demoting it to prepared.
			agent.AdapterLoaded = reg.Agents[i].AdapterLoaded
			agent.ConnectionStatus = reg.Agents[i].ConnectionStatus
			agent.ConnectionMode = reg.Agents[i].ConnectionMode
			agent.ConnectionID = reg.Agents[i].ConnectionID
			agent.ConnectedAt = reg.Agents[i].ConnectedAt
			agent.LastError = reg.Agents[i].LastError
			reg.Agents[i] = agent
			replaced = true
			break
		}
	}
	if !replaced {
		reg.Agents = append(reg.Agents, agent)
	}
	return writeJSON(filepath.Join(root, ".brain2", "agents.json"), reg)
}

func safeID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '\u4e00' && r <= '\u9fff')
		if valid {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = "project-" + time.Now().Format("20060102150405")
	}
	return id
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), 0o644)
}

func writeJSONIfMissing(path string, v any) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeJSON(path, v)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".brain2-write-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.Rename(tmpPath, path)
	}
	backup := path + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(path, backup); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
