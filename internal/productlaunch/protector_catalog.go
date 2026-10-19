package productlaunch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"meow-launcher/internal/brain2"
)

const (
	CatalogContractVersion       = "brain2-project-catalog/v1"
	LegacyCatalogContractVersion = "0.1.0-catalog-fixture"
)

type ProjectCatalogEntry struct {
	ProjectID       string     `json:"project_id"`
	DisplayName     string     `json:"display_name"`
	EntityKind      string     `json:"entity_kind"`
	LifecycleStage  string     `json:"lifecycle_stage"`
	RepositoryReady bool       `json:"repository_ready"`
	DefaultBranch   string     `json:"default_branch,omitempty"`
	DeviceID        string     `json:"device_id"`
	PathFingerprint string     `json:"path_fingerprint"`
	LaunchTask      LaunchTask `json:"launch_task"`
}

type ProjectCatalogTask struct {
	ContractVersion string                `json:"contract_version"`
	CatalogID       string                `json:"catalog_id"`
	Nonce           string                `json:"nonce"`
	IssuedAt        string                `json:"issued_at"`
	ExpiresAt       string                `json:"expires_at"`
	ProductID       string                `json:"product_id"`
	ProductVersion  string                `json:"product_version"`
	RegistrySchema  string                `json:"registry_schema"`
	DeviceID        string                `json:"device_id"`
	Projects        []ProjectCatalogEntry `json:"projects"`
}

type PreparedCatalogLaunch struct {
	CatalogID    string
	DeviceID     string
	CatalogPath  string
	ReceiptPath  string
	Command      string
	Args         []string
	ProjectCount int
	ExpiresAt    string
}

type CatalogHandshake struct {
	ContractVersion string  `json:"contract_version"`
	CatalogID       string  `json:"catalog_id"`
	ProductID       string  `json:"product_id"`
	ProductVersion  string  `json:"product_version"`
	RegistrySchema  string  `json:"registry_schema"`
	DeviceID        string  `json:"device_id"`
	ProjectCount    int     `json:"project_count"`
	Status          string  `json:"status"`
	ErrorCode       *string `json:"error_code"`
}

type catalogHandshakeReceipt struct {
	Handshake  CatalogHandshake `json:"handshake"`
	RecordedAt string           `json:"recorded_at"`
}

func catalogLaunchTask(workspaceID, localDeviceID, localMachineName string, project brain2.Project, issuedAt time.Time) (LaunchTask, error) {
	if project.Status != "active" || project.PathStatus != "verified" || project.TruthStatus != "user_confirmed" {
		return LaunchTask{}, errors.New("项目未达到 Protector 目录交付条件")
	}
	asset, canonical, err := chooseAsset(project, localDeviceID, localMachineName)
	if err != nil {
		return LaunchTask{}, err
	}
	taskID, err := randomID("plt_")
	if err != nil {
		return LaunchTask{}, err
	}
	nonce, err := randomID("")
	if err != nil {
		return LaunchTask{}, err
	}
	grantID, err := randomID("pgrant_")
	if err != nil {
		return LaunchTask{}, err
	}
	expiresAt := issuedAt.Add(30 * time.Minute)
	scopes := []string{"metadata", "read", "scan"}
	return LaunchTask{
		ContractVersion: ContractVersion, TaskID: taskID, Nonce: nonce,
		IssuedAt: issuedAt.Format(time.RFC3339Nano), ExpiresAt: expiresAt.Format(time.RFC3339Nano),
		ProductID: ProductID, ProductVersion: ProductVersion, RequestedScopes: scopes,
		ProjectRef: buildProjectRef(workspaceID, localDeviceID, project, asset, canonical),
		ProductGrant: ProductGrant{
			GrantVersion: ContractVersion, GrantID: grantID, ProductID: ProductID,
			ProjectID: project.ID, AssetIDs: []string{asset.ID}, Scopes: scopes,
			DeviceID: localDeviceID, GrantedAt: issuedAt.Format(time.RFC3339Nano),
			ExpiresAt: expiresAt.Format(time.RFC3339Nano), RevokedAt: nil,
			Activation: "protector-user-confirmation-required",
		},
	}, nil
}

func PrepareProtectorCatalogLaunch(dataRoot, workspaceID, productRoot, localDeviceID, localMachineName string, projects []brain2.Project, now time.Time) (PreparedCatalogLaunch, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return PreparedCatalogLaunch{}, errors.New("Brain2 workspace ID 缺失")
	}
	localDeviceID = strings.TrimSpace(localDeviceID)
	if localDeviceID == "" {
		return PreparedCatalogLaunch{}, errors.New("本机 Brain2 设备 ID 缺失")
	}
	command, err := protectorCommand(productRoot)
	if err != nil {
		return PreparedCatalogLaunch{}, err
	}
	dataRoot = filepath.Clean(strings.TrimSpace(dataRoot))
	if dataRoot == "." || !filepath.IsAbs(dataRoot) {
		return PreparedCatalogLaunch{}, errors.New("Launcher 数据目录必须是绝对路径")
	}
	issuedAt := now.UTC()
	entries := make([]ProjectCatalogEntry, 0, len(projects))
	seen := map[string]bool{}
	for _, project := range projects {
		if seen[project.ID] {
			return PreparedCatalogLaunch{}, errors.New("Protector 项目目录包含重复项目 ID")
		}
		task, err := catalogLaunchTask(workspaceID, localDeviceID, localMachineName, project, issuedAt)
		if err != nil {
			return PreparedCatalogLaunch{}, fmt.Errorf("项目 %s 不可交付给 Protector: %w", project.Name, err)
		}
		seen[project.ID] = true
		entries = append(entries, ProjectCatalogEntry{
			ProjectID: project.ID, DisplayName: project.Name,
			EntityKind: project.EntityKind, LifecycleStage: project.LifecycleStage,
			RepositoryReady: strings.TrimSpace(project.RepositoryURL) != "", DefaultBranch: project.DefaultBranch,
			DeviceID:        localDeviceID,
			PathFingerprint: task.ProjectRef.PathFingerprint, LaunchTask: task,
		})
	}
	if len(entries) == 0 {
		return PreparedCatalogLaunch{}, errors.New("当前 Brain2 没有可交付给 Protector 的已确认项目")
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].DisplayName) < strings.ToLower(entries[j].DisplayName)
	})
	catalogID, err := randomID("pcat_")
	if err != nil {
		return PreparedCatalogLaunch{}, err
	}
	nonce, err := randomID("")
	if err != nil {
		return PreparedCatalogLaunch{}, err
	}
	expiresAt := issuedAt.Add(30 * time.Minute)
	catalog := ProjectCatalogTask{
		ContractVersion: CatalogContractVersion, CatalogID: catalogID, Nonce: nonce,
		IssuedAt: issuedAt.Format(time.RFC3339Nano), ExpiresAt: expiresAt.Format(time.RFC3339Nano),
		ProductID: ProductID, ProductVersion: ProductVersion,
		RegistrySchema: brain2.ProjectRegistrySchemaVersion, DeviceID: localDeviceID,
		Projects: entries,
	}
	launchRoot := filepath.Join(dataRoot, "runtime", "product-catalogs", catalogID)
	catalogPath := filepath.Join(launchRoot, "project-catalog.json")
	receiptPath := filepath.Join(launchRoot, "catalog-handshake.json")
	if err := writeJSON(catalogPath, catalog); err != nil {
		return PreparedCatalogLaunch{}, fmt.Errorf("写入 Protector 项目目录失败: %w", err)
	}
	return PreparedCatalogLaunch{
		CatalogID: catalogID, DeviceID: localDeviceID, CatalogPath: catalogPath, ReceiptPath: receiptPath,
		Command: command, Args: []string{filepath.Clean(productRoot), "--project-catalog", catalogPath, "--catalog-receipt", receiptPath},
		ProjectCount: len(entries), ExpiresAt: expiresAt.Format(time.RFC3339Nano),
	}, nil
}

func AwaitCatalogHandshake(prepared PreparedCatalogLaunch, timeout time.Duration) (CatalogHandshake, error) {
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(prepared.ReceiptPath)
		if err == nil {
			if len(data) > 64*1024 {
				return CatalogHandshake{}, errors.New("Protector 目录握手回执超过安全大小限制")
			}
			var receipt catalogHandshakeReceipt
			if err := json.Unmarshal(data, &receipt); err != nil {
				return CatalogHandshake{}, errors.New("Protector 目录握手回执不是有效 JSON")
			}
			handshake := receipt.Handshake
			if handshake.ContractVersion != CatalogContractVersion || handshake.CatalogID != prepared.CatalogID ||
				handshake.ProductID != ProductID || handshake.ProductVersion != ProductVersion ||
				handshake.RegistrySchema != brain2.ProjectRegistrySchemaVersion ||
				handshake.DeviceID != prepared.DeviceID || handshake.ProjectCount != prepared.ProjectCount {
				return CatalogHandshake{}, errors.New("Protector 目录握手身份与本次启动不一致")
			}
			if handshake.Status != "accepted" {
				return CatalogHandshake{}, errors.New("Protector 拒绝本次项目目录")
			}
			return handshake, nil
		}
		if !os.IsNotExist(err) {
			return CatalogHandshake{}, fmt.Errorf("读取 Protector 目录握手失败: %w", err)
		}
		if time.Now().After(deadline) {
			return CatalogHandshake{}, errors.New("等待 Protector 项目目录握手超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
