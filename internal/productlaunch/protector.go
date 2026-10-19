package productlaunch

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"meow-launcher/internal/brain2"
)

const (
	ContractVersion       = "brain2-product-launch/v1"
	LegacyContractVersion = "0.1.0-fixture"
	ProductID             = "meow-protector"
	ProductVersion        = "0.1.0-gate-e"
)

type ProjectRef struct {
	RegistrySchema     string `json:"registry_schema"`
	WorkspaceID        string `json:"workspace_id"`
	DeviceID           string `json:"device_id"`
	ProjectID          string `json:"project_id"`
	ProjectName        string `json:"project_name"`
	EntityKind         string `json:"entity_kind"`
	ParentProjectID    string `json:"parent_project_id,omitempty"`
	LifecycleStage     string `json:"lifecycle_stage"`
	Brain2Entry        string `json:"brain2_entry,omitempty"`
	RepositoryURL      string `json:"repository_url,omitempty"`
	DefaultBranch      string `json:"default_branch,omitempty"`
	AssetID            string `json:"asset_id"`
	AssetDeviceID      string `json:"asset_device_id,omitempty"`
	SourcePathSnapshot string `json:"source_path_snapshot"`
	PathFingerprint    string `json:"path_fingerprint"`
	PathStatus         string `json:"path_status"`
	TruthStatus        string `json:"truth_status"`
	ConnectionGate     string `json:"connection_gate,omitempty"`
}

type ProductGrant struct {
	GrantVersion string   `json:"grant_version"`
	GrantID      string   `json:"grant_id"`
	ProductID    string   `json:"product_id"`
	ProjectID    string   `json:"project_id"`
	AssetIDs     []string `json:"asset_ids"`
	Scopes       []string `json:"scopes"`
	DeviceID     string   `json:"device_id"`
	GrantedAt    string   `json:"granted_at"`
	ExpiresAt    string   `json:"expires_at"`
	RevokedAt    *string  `json:"revoked_at"`
	Activation   string   `json:"activation,omitempty"`
}

type LaunchTask struct {
	ContractVersion string       `json:"contract_version"`
	TaskID          string       `json:"task_id"`
	Nonce           string       `json:"nonce"`
	IssuedAt        string       `json:"issued_at"`
	ExpiresAt       string       `json:"expires_at"`
	ProductID       string       `json:"product_id"`
	ProductVersion  string       `json:"product_version"`
	RequestedScopes []string     `json:"requested_scopes"`
	ProjectRef      ProjectRef   `json:"project_ref"`
	ProductGrant    ProductGrant `json:"product_grant"`
}

type PreparedLaunch struct {
	TaskID          string
	ProjectID       string
	DeviceID        string
	TaskPath        string
	ReceiptPath     string
	PathFingerprint string
	Command         string
	Args            []string
	ExpiresAt       string
}

type ProductHandshake struct {
	ContractVersion string   `json:"contract_version"`
	TaskID          string   `json:"task_id"`
	ProductID       string   `json:"product_id"`
	ProductVersion  string   `json:"product_version"`
	RegistrySchema  string   `json:"registry_schema"`
	DeviceID        string   `json:"device_id"`
	ProjectID       string   `json:"project_id"`
	PathFingerprint string   `json:"path_fingerprint"`
	AcceptedScopes  []string `json:"accepted_scopes"`
	Status          string   `json:"status"`
	ErrorCode       *string  `json:"error_code"`
}

type productHandshakeReceipt struct {
	Handshake  ProductHandshake `json:"handshake"`
	RecordedAt string           `json:"recorded_at"`
}

func randomID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value), nil
}

func canonicalPath(value string) (string, error) {
	value = filepath.Clean(strings.TrimSpace(value))
	if value == "." || !filepath.IsAbs(value) {
		return "", errors.New("项目路径必须是绝对路径")
	}
	canonical, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", errors.New("项目路径不可用")
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", errors.New("项目路径不可用或不是目录")
	}
	return filepath.Clean(canonical), nil
}

func pathFingerprint(value string) string {
	identity := value
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
	}
	digest := sha256.Sum256([]byte(identity))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func protectorCommand(productRoot string) (string, error) {
	productRoot = filepath.Clean(strings.TrimSpace(productRoot))
	if productRoot == "." || !filepath.IsAbs(productRoot) {
		return "", errors.New("Protector 开发联动目录未配置")
	}
	for _, required := range []string{
		filepath.Join(productRoot, "package.json"),
		filepath.Join(productRoot, "desktop", "main.cjs"),
	} {
		if info, err := os.Stat(required); err != nil || !info.Mode().IsRegular() {
			return "", errors.New("Protector 开发联动目录不完整")
		}
	}
	command := filepath.Join(productRoot, "node_modules", "electron", "dist", "electron.exe")
	if info, err := os.Stat(command); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("Protector Electron 运行时不可用")
	}
	return command, nil
}

func Available(productRoot string) bool {
	_, err := protectorCommand(productRoot)
	return err == nil
}

func chooseAsset(project brain2.Project, localDeviceID, localMachineName string) (brain2.ProjectAsset, string, error) {
	localDeviceID = strings.TrimSpace(localDeviceID)
	localMachineName = strings.TrimSpace(localMachineName)
	var sourceCanonical string
	if project.SourcePath != nil {
		sourceCanonical, _ = canonicalPath(*project.SourcePath)
	}
	var fallback *brain2.ProjectAsset
	for index := range project.Assets {
		asset := project.Assets[index]
		if asset.Status != "verified" || asset.PathKind != "absolute" {
			continue
		}
		canonical, err := canonicalPath(asset.Path)
		if err != nil {
			continue
		}
		assetDeviceID := strings.TrimSpace(asset.DeviceID)
		if (localDeviceID != "" && strings.EqualFold(assetDeviceID, localDeviceID)) ||
			(localMachineName != "" && strings.EqualFold(assetDeviceID, localMachineName)) {
			return asset, canonical, nil
		}
		if sourceCanonical != "" && strings.EqualFold(canonical, sourceCanonical) {
			candidate := asset
			fallback = &candidate
		}
	}
	if fallback != nil {
		return *fallback, sourceCanonical, nil
	}
	return brain2.ProjectAsset{}, "", errors.New("项目没有当前设备可用且已验证的本地资产")
}

func ProjectAvailableOnDevice(project brain2.Project, localDeviceID, localMachineName string) bool {
	if project.Status != "active" || project.PathStatus != "verified" || project.TruthStatus != "user_confirmed" {
		return false
	}
	_, _, err := chooseAsset(project, localDeviceID, localMachineName)
	return err == nil
}

func ProjectPathOnDevice(project brain2.Project, deviceID, machineName string) (string, bool) {
	_, canonical, err := chooseAsset(project, deviceID, machineName)
	return canonical, err == nil
}

func buildProjectRef(workspaceID, localDeviceID string, project brain2.Project, asset brain2.ProjectAsset, canonical string) ProjectRef {
	entityKind := strings.TrimSpace(project.EntityKind)
	if entityKind == "" {
		entityKind = "needs_verification"
	}
	lifecycleStage := strings.TrimSpace(project.LifecycleStage)
	if lifecycleStage == "" {
		lifecycleStage = "needs_verification"
	}
	return ProjectRef{
		RegistrySchema: brain2.ProjectRegistrySchemaVersion,
		WorkspaceID:    workspaceID, DeviceID: localDeviceID,
		ProjectID: project.ID, ProjectName: project.Name,
		EntityKind: entityKind, ParentProjectID: project.ParentProjectID,
		LifecycleStage: lifecycleStage, Brain2Entry: project.Brain2Entry,
		RepositoryURL: project.RepositoryURL, DefaultBranch: project.DefaultBranch,
		AssetID: asset.ID, AssetDeviceID: asset.DeviceID,
		SourcePathSnapshot: canonical, PathFingerprint: pathFingerprint(canonical),
		PathStatus: "confirmed", TruthStatus: "user-confirmed",
		ConnectionGate: project.ConnectionGate,
	}
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".protector-launch-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func AwaitProtectorHandshake(prepared PreparedLaunch, timeout time.Duration) (ProductHandshake, error) {
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(prepared.ReceiptPath)
		if err == nil {
			if len(data) > 64*1024 {
				return ProductHandshake{}, errors.New("Protector 握手回执超过安全大小限制")
			}
			var receipt productHandshakeReceipt
			if err := json.Unmarshal(data, &receipt); err != nil {
				return ProductHandshake{}, errors.New("Protector 握手回执不是有效 JSON")
			}
			handshake := receipt.Handshake
			if handshake.ContractVersion != ContractVersion || handshake.TaskID != prepared.TaskID ||
				handshake.ProductID != ProductID || handshake.ProductVersion != ProductVersion ||
				handshake.RegistrySchema != brain2.ProjectRegistrySchemaVersion ||
				handshake.DeviceID != prepared.DeviceID || handshake.ProjectID != prepared.ProjectID {
				return ProductHandshake{}, errors.New("Protector 握手身份与本次启动任务不一致")
			}
			if handshake.Status != "accepted" {
				code := "UNKNOWN_REJECTION"
				if handshake.ErrorCode != nil && strings.TrimSpace(*handshake.ErrorCode) != "" {
					code = strings.TrimSpace(*handshake.ErrorCode)
				}
				return ProductHandshake{}, fmt.Errorf("Protector 拒绝本次任务：%s", code)
			}
			if handshake.PathFingerprint != prepared.PathFingerprint || len(handshake.AcceptedScopes) != 3 ||
				handshake.AcceptedScopes[0] != "metadata" || handshake.AcceptedScopes[1] != "read" || handshake.AcceptedScopes[2] != "scan" {
				return ProductHandshake{}, errors.New("Protector 握手未确认完整只读扫描范围")
			}
			return handshake, nil
		}
		if !os.IsNotExist(err) {
			return ProductHandshake{}, fmt.Errorf("读取 Protector 握手回执失败: %w", err)
		}
		if time.Now().After(deadline) {
			return ProductHandshake{}, errors.New("等待 Protector 握手超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func PrepareProtectorLaunch(dataRoot, workspaceID, productRoot, localDeviceID, localMachineName string, project brain2.Project, now time.Time) (PreparedLaunch, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return PreparedLaunch{}, errors.New("Brain2 workspace ID 缺失")
	}
	if project.Status != "active" || project.PathStatus != "verified" || project.TruthStatus != "user_confirmed" {
		return PreparedLaunch{}, errors.New("只有用户确认且路径已验证的项目可以授权 Protector")
	}
	localDeviceID = strings.TrimSpace(localDeviceID)
	if localDeviceID == "" {
		return PreparedLaunch{}, errors.New("本机 Brain2 设备 ID 缺失")
	}
	asset, canonical, err := chooseAsset(project, localDeviceID, localMachineName)
	if err != nil {
		return PreparedLaunch{}, err
	}
	command, err := protectorCommand(productRoot)
	if err != nil {
		return PreparedLaunch{}, err
	}
	dataRoot = filepath.Clean(strings.TrimSpace(dataRoot))
	if dataRoot == "." || !filepath.IsAbs(dataRoot) {
		return PreparedLaunch{}, errors.New("Launcher 数据目录必须是绝对路径")
	}
	taskID, err := randomID("plt_")
	if err != nil {
		return PreparedLaunch{}, err
	}
	nonce, err := randomID("")
	if err != nil {
		return PreparedLaunch{}, err
	}
	grantID, err := randomID("pgrant_")
	if err != nil {
		return PreparedLaunch{}, err
	}
	issuedAt := now.UTC()
	taskExpires := issuedAt.Add(5 * time.Minute)
	grantExpires := issuedAt.Add(10 * time.Minute)
	scopes := []string{"metadata", "read", "scan"}
	task := LaunchTask{
		ContractVersion: ContractVersion, TaskID: taskID, Nonce: nonce,
		IssuedAt: issuedAt.Format(time.RFC3339Nano), ExpiresAt: taskExpires.Format(time.RFC3339Nano),
		ProductID: ProductID, ProductVersion: ProductVersion, RequestedScopes: scopes,
		ProjectRef: buildProjectRef(workspaceID, localDeviceID, project, asset, canonical),
		ProductGrant: ProductGrant{
			GrantVersion: ContractVersion, GrantID: grantID, ProductID: ProductID,
			ProjectID: project.ID, AssetIDs: []string{asset.ID}, Scopes: scopes,
			DeviceID: localDeviceID, GrantedAt: issuedAt.Format(time.RFC3339Nano),
			ExpiresAt: grantExpires.Format(time.RFC3339Nano), RevokedAt: nil,
			Activation: "launcher-user-confirmed",
		},
	}
	launchRoot := filepath.Join(dataRoot, "runtime", "product-launches", taskID)
	taskPath := filepath.Join(launchRoot, "launch-task.json")
	receiptPath := filepath.Join(launchRoot, "product-handshake.json")
	if err := writeJSON(taskPath, task); err != nil {
		return PreparedLaunch{}, fmt.Errorf("写入 Protector 启动任务失败: %w", err)
	}
	return PreparedLaunch{
		TaskID: taskID, ProjectID: project.ID, DeviceID: localDeviceID, TaskPath: taskPath, ReceiptPath: receiptPath,
		PathFingerprint: task.ProjectRef.PathFingerprint,
		Command:         command,
		Args:            []string{filepath.Clean(productRoot), "--launch-task", taskPath, "--launch-receipt", receiptPath},
		ExpiresAt:       taskExpires.Format(time.RFC3339Nano),
	}, nil
}
