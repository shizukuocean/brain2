package brain2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	ProjectRegistrySchemaVersion = "0.3"
	projectManifestVersion       = "0.1"
	projectManifestFilename      = "项目资产登记.md"
	machineProjectRecordVersion  = "0.1"
	machineProjectCapability     = "machine_project_record"
)

type MachineProjectDiscovery struct {
	ScanTaskID         string   `json:"scan_task_id"`
	AgentID            string   `json:"agent_id"`
	Confidence         float64  `json:"confidence"`
	Evidence           []string `json:"evidence"`
	DetectedKind       string   `json:"detected_kind,omitempty"`
	SuggestedConsumers []string `json:"suggested_consumers,omitempty"`
}

type MachineProjectRecord struct {
	SchemaVersion string                  `json:"schema_version"`
	RecordKind    string                  `json:"record_kind"`
	Project       Project                 `json:"project"`
	Discovery     MachineProjectDiscovery `json:"discovery"`
}

type ProjectAsset struct {
	ID        string   `json:"id"`
	Role      string   `json:"role"`
	Path      string   `json:"path"`
	PathKind  string   `json:"path_kind"`
	DeviceID  string   `json:"device_id,omitempty"`
	Status    string   `json:"status"`
	Truth     string   `json:"truth"`
	Access    string   `json:"access"`
	Consumers []string `json:"consumers,omitempty"`
}

func newManagedProject(root string, project Project, assetPath, role, pathKind string) Project {
	project.ManifestPath = filepath.ToSlash(filepath.Join(filepath.Dir(project.Brain2Entry), projectManifestFilename))
	project.ManifestVersion = projectManifestVersion
	project.AssetStatus = "registered"
	project.PathStatus = "verified"
	project.TruthStatus = "user_confirmed"
	project.ConnectionGate = "user_confirmation_required"
	project.Capabilities = []string{"project_manifest", "path_provider"}
	if project.SharedRepositoryProjectID != "" {
		project.Capabilities = append(project.Capabilities, "shared_repository")
	}
	deviceID := "portable"
	if pathKind == "absolute" {
		deviceID = workspaceDeviceID(root)
	}
	project.Assets = []ProjectAsset{{
		ID: "primary-root", Role: role, Path: assetPath, PathKind: pathKind,
		DeviceID: deviceID, Status: "verified", Truth: "current", Access: "metadata_only",
		Consumers: []string{"meow-blueprint", "meow-protector", "dev-helper"},
	}}
	if project.RepositoryURL != "" {
		project.Capabilities = append(project.Capabilities, "repository_provider")
		project.Assets = append(project.Assets, ProjectAsset{
			ID: "source-repository", Role: "source_repository", Path: project.RepositoryURL, PathKind: "url",
			DeviceID: "portable", Status: "verified", Truth: "current", Access: "metadata_only",
			Consumers: []string{"meow-blueprint", "meow-protector", "dev-helper"},
		})
	}
	return project
}

func upsertManagedDeviceAsset(project Project, assetPath, role, pathKind, deviceID string) Project {
	project.ManifestPath = filepath.ToSlash(filepath.Join(filepath.Dir(project.Brain2Entry), projectManifestFilename))
	project.ManifestVersion = projectManifestVersion
	project.AssetStatus = "registered"
	project.PathStatus = "verified"
	project.TruthStatus = "user_confirmed"
	project.ConnectionGate = "user_confirmation_required"
	project.Capabilities = []string{"project_manifest", "path_provider"}
	if project.SharedRepositoryProjectID != "" {
		project.Capabilities = append(project.Capabilities, "shared_repository")
	}
	if project.RepositoryURL != "" {
		project.Capabilities = append(project.Capabilities, "repository_provider")
	}

	deviceID = strings.ToUpper(strings.TrimSpace(deviceID))
	for index := range project.Assets {
		asset := &project.Assets[index]
		if asset.PathKind == "absolute" {
			if _, err := NormalizeDeviceID(asset.DeviceID); err != nil {
				asset.DeviceID = InferLegacyDeviceID(asset.Path, asset.DeviceID)
			}
		}
	}

	assetID := "primary-root"
	if deviceID != "" {
		assetID = "device-" + strings.ToLower(deviceID) + "-root"
	}
	replacement := ProjectAsset{
		ID: assetID, Role: role, Path: assetPath, PathKind: pathKind,
		DeviceID: deviceID, Status: "verified", Truth: "current", Access: "metadata_only",
		Consumers: []string{"meow-blueprint", "meow-protector", "dev-helper"},
	}
	updated := false
	for index := range project.Assets {
		asset := project.Assets[index]
		if asset.PathKind == pathKind && strings.EqualFold(strings.TrimSpace(asset.DeviceID), deviceID) && asset.Role == role {
			replacement.ID = asset.ID
			project.Assets[index] = replacement
			updated = true
			break
		}
	}
	if !updated {
		project.Assets = append(project.Assets, replacement)
	}

	if project.RepositoryURL != "" {
		hasRepository := false
		for index := range project.Assets {
			asset := project.Assets[index]
			if asset.PathKind == "url" && strings.EqualFold(asset.Path, project.RepositoryURL) {
				hasRepository = true
				break
			}
		}
		if !hasRepository {
			project.Assets = append(project.Assets, ProjectAsset{
				ID: "source-repository", Role: "source_repository", Path: project.RepositoryURL, PathKind: "url",
				DeviceID: "portable", Status: "verified", Truth: "current", Access: "metadata_only",
				Consumers: []string{"meow-blueprint", "meow-protector", "dev-helper"},
			})
		}
	}
	return project
}

// newMachineRegisteredProject creates only Launcher-owned machine metadata.
// It deliberately does not create, move, or edit any user-facing Brain2 note.
func newMachineRegisteredProject(root string, project Project, assetPath string) Project {
	project.Brain2Entry = filepath.ToSlash(filepath.Join(".brain2", "project-records", project.ID+".json"))
	project.ManifestPath = ""
	project.ManifestVersion = ""
	project.AssetStatus = "registered"
	project.PathStatus = "verified"
	project.TruthStatus = "user_confirmed"
	project.ConnectionGate = "separate_product_authorization_required"
	project.Capabilities = []string{machineProjectCapability, "path_provider"}
	project.Assets = []ProjectAsset{{
		ID: "primary-root", Role: "source_root", Path: assetPath, PathKind: "absolute",
		DeviceID: workspaceDeviceID(root), Status: "verified", Truth: "current", Access: "metadata_only",
	}}
	return project
}

func writeMachineProjectRecord(root string, project Project, candidate ProjectCandidate) error {
	record := MachineProjectRecord{
		SchemaVersion: machineProjectRecordVersion,
		RecordKind:    "launcher_project_registration",
		Project:       project,
		Discovery: MachineProjectDiscovery{
			ScanTaskID: candidate.ScanTaskID, AgentID: candidate.AgentID,
			Confidence: candidate.Confidence, Evidence: candidate.Evidence,
			DetectedKind: candidate.DetectedKind, SuggestedConsumers: candidate.SuggestedConsumers,
		},
	}
	return writeJSON(filepath.Join(root, filepath.FromSlash(project.Brain2Entry)), record)
}

func hasProjectCapability(project Project, capability string) bool {
	for _, value := range project.Capabilities {
		if value == capability {
			return true
		}
	}
	return false
}

func workspaceDeviceID(root string) string {
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "config.json"))
	if err == nil {
		var cfg Config
		if json.Unmarshal(data, &cfg) == nil && strings.TrimSpace(cfg.DeviceName) != "" {
			return strings.TrimSpace(cfg.DeviceName)
		}
	}
	device, _ := os.Hostname()
	if strings.TrimSpace(device) == "" {
		return "unknown-device"
	}
	return strings.TrimSpace(device)
}

func writeProjectManifest(root string, project Project) error {
	if strings.TrimSpace(project.ManifestPath) == "" {
		return fmt.Errorf("项目 %s 缺少 manifest_path", project.ID)
	}
	return atomicWrite(filepath.Join(root, filepath.FromSlash(project.ManifestPath)), []byte(renderProjectManifest(project)), 0o644)
}

func renderProjectManifest(project Project) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "brain2_project_manifest: %s\n", strconv.Quote(project.ManifestVersion))
	fmt.Fprintf(&b, "registry_schema: %s\n", strconv.Quote(ProjectRegistrySchemaVersion))
	fmt.Fprintf(&b, "project_id: %s\n", strconv.Quote(project.ID))
	fmt.Fprintf(&b, "project_name: %s\n", strconv.Quote(project.Name))
	fmt.Fprintf(&b, "entity_kind: %s\n", strconv.Quote(project.EntityKind))
	if project.ParentProjectID != "" {
		fmt.Fprintf(&b, "parent_project_id: %s\n", strconv.Quote(project.ParentProjectID))
	}
	if project.SharedRepositoryProjectID != "" {
		fmt.Fprintf(&b, "shared_repository_project_id: %s\n", strconv.Quote(project.SharedRepositoryProjectID))
	}
	fmt.Fprintf(&b, "lifecycle_stage: %s\n", strconv.Quote(project.LifecycleStage))
	fmt.Fprintf(&b, "project_mode: %s\n", strconv.Quote(project.Mode))
	fmt.Fprintf(&b, "project_status: %s\n", strconv.Quote(project.Status))
	fmt.Fprintf(&b, "asset_status: %s\n", strconv.Quote(project.AssetStatus))
	fmt.Fprintf(&b, "path_status: %s\n", strconv.Quote(project.PathStatus))
	fmt.Fprintf(&b, "truth_status: %s\n", strconv.Quote(project.TruthStatus))
	fmt.Fprintf(&b, "connection_gate: %s\n", strconv.Quote(project.ConnectionGate))
	fmt.Fprintf(&b, "updated_at: %s\n", strconv.Quote(project.UpdatedAt))
	fmt.Fprintf(&b, "source: %s\n", strconv.Quote("Meow Launcher"))
	fmt.Fprintf(&b, "---\n\n# 项目资产登记\n\n")
	fmt.Fprintf(&b, "> 项目：[[%s|%s]]  \n", strings.TrimSuffix(project.Brain2Entry, ".md"), project.Name)
	fmt.Fprintf(&b, "> 状态：当前项目身份证；路径变更应通过 Launcher 重新定位并同步机器索引。\n\n")
	fmt.Fprintf(&b, "## 身份\n\n")
	fmt.Fprintf(&b, "| 字段 | 值 |\n|---|---|\n")
	fmt.Fprintf(&b, "| Project ID | `%s` |\n", markdownCell(project.ID))
	fmt.Fprintf(&b, "| 对象类型 | `%s` |\n", markdownCell(project.EntityKind))
	fmt.Fprintf(&b, "| 生命周期 | `%s` |\n", markdownCell(project.LifecycleStage))
	if project.ParentProjectID != "" {
		fmt.Fprintf(&b, "| 父项目 | `%s` |\n", markdownCell(project.ParentProjectID))
	}
	if project.SharedRepositoryProjectID != "" {
		fmt.Fprintf(&b, "| 共享代码仓库 | `%s`（独立产品，不是父子项目） |\n", markdownCell(project.SharedRepositoryProjectID))
	}
	fmt.Fprintf(&b, "| 模式 | `%s` |\n", markdownCell(project.Mode))
	fmt.Fprintf(&b, "| Brain2 入口 | `%s` |\n", markdownCell(project.Brain2Entry))
	fmt.Fprintf(&b, "| 资产状态 | `%s` |\n", markdownCell(project.AssetStatus))
	fmt.Fprintf(&b, "| 路径状态 | `%s` |\n", markdownCell(project.PathStatus))
	fmt.Fprintf(&b, "| 真值状态 | `%s` |\n", markdownCell(project.TruthStatus))
	fmt.Fprintf(&b, "| 产品连接门 | `%s` |\n\n", markdownCell(project.ConnectionGate))
	fmt.Fprintf(&b, "## 当前资产\n\n")
	fmt.Fprintf(&b, "| ID | 角色 | 路径 | 路径类型 | 设备 | 状态 | 真值 | 访问 |\n|---|---|---|---|---|---|---|---|\n")
	for _, asset := range project.Assets {
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n",
			markdownCell(asset.ID), markdownCell(asset.Role), markdownCell(asset.Path), markdownCell(asset.PathKind),
			markdownCell(asset.DeviceID), markdownCell(asset.Status), markdownCell(asset.Truth), markdownCell(asset.Access))
	}
	if project.RepositoryURL != "" {
		fmt.Fprintf(&b, "\n## 代码真值\n\n")
		fmt.Fprintf(&b, "- GitHub：[%s](%s)\n", markdownCell(project.RepositoryURL), project.RepositoryURL)
		if project.DefaultBranch != "" {
			fmt.Fprintf(&b, "- 默认分支：`%s`\n", markdownCell(project.DefaultBranch))
		}
	}
	fmt.Fprintf(&b, "\n## 下游读取边界\n\n")
	fmt.Fprintf(&b, "- Meow Blueprint、Meow Protector 与 Dev Helper 可以只读发现本文件和 `.brain2/projects.json` 中的项目元数据。\n")
	fmt.Fprintf(&b, "- `registered` / `verified` 不代表自动授权扫描或修改；具体项目和范围仍需用户确认。\n")
	fmt.Fprintf(&b, "- 不得因发现本项目而读取未登记资产、私人笔记、客户资料或敏感数据。\n\n")
	fmt.Fprintf(&b, "## 变更规则\n\n")
	fmt.Fprintf(&b, "- 本文件是项目级路径与资产的治理真值。\n")
	fmt.Fprintf(&b, "- `.brain2/projects.json` 是 Launcher 与下游产品使用的机器索引。\n")
	fmt.Fprintf(&b, "- 两者不一致时健康检查必须报警，不得静默猜测。\n")
	return b.String()
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "`", "'")
}

func readManifestScalars(content string) map[string]string {
	values := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return values
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, raw, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		raw = strings.TrimSpace(raw)
		if unquoted, err := strconv.Unquote(raw); err == nil {
			values[key] = unquoted
		} else {
			values[key] = raw
		}
	}
	return values
}
