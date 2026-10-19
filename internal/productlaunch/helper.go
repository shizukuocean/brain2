package productlaunch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"meow-launcher/internal/brain2"
)

const (
	HelperContractVersion = ContractVersion
	HelperProductID       = "dev-helper"
	HelperProductVersion  = "0.3.0-p1"
)

var helperScopes = []string{"metadata", "read", "candidate_write"}

type HelperProductGrant struct {
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

type HelperLaunchTask struct {
	ContractVersion string             `json:"contract_version"`
	TaskID          string             `json:"task_id"`
	Nonce           string             `json:"nonce"`
	IssuedAt        string             `json:"issued_at"`
	ExpiresAt       string             `json:"expires_at"`
	ProductID       string             `json:"product_id"`
	ProductVersion  string             `json:"product_version"`
	RequestedScopes []string           `json:"requested_scopes"`
	ProjectRef      ProjectRef         `json:"project_ref"`
	ProductGrant    HelperProductGrant `json:"product_grant"`
}

type PreparedHelperLaunch struct {
	TaskID          string
	ProjectID       string
	DeviceID        string
	TaskPath        string
	ReceiptPath     string
	PathFingerprint string
	RequestedScopes []string
	Command         string
	Args            []string
	ExpiresAt       string
}

type HelperProductHandshake struct {
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

type helperHandshakeReceipt struct {
	Handshake  HelperProductHandshake `json:"handshake"`
	RecordedAt string                 `json:"recorded_at"`
}

func helperCommand(productRoot string) (string, error) {
	productRoot = filepath.Clean(strings.TrimSpace(productRoot))
	if productRoot == "." || !filepath.IsAbs(productRoot) {
		return "", errors.New("Dev Helper 开发联动目录未配置")
	}
	for _, required := range []string{
		filepath.Join(productRoot, "package.json"),
		filepath.Join(productRoot, "desktop", "main.js"),
	} {
		if info, err := os.Stat(required); err != nil || !info.Mode().IsRegular() {
			return "", errors.New("Dev Helper 开发联动目录不完整")
		}
	}
	command := filepath.Join(productRoot, "node_modules", "electron", "dist", "electron.exe")
	if info, err := os.Stat(command); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("Dev Helper Electron 运行时不可用")
	}
	return command, nil
}

func HelperAvailable(productRoot string) bool {
	_, err := helperCommand(productRoot)
	return err == nil
}

func AwaitHelperHandshake(prepared PreparedHelperLaunch, timeout time.Duration) (HelperProductHandshake, error) {
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(prepared.ReceiptPath)
		if err == nil {
			if len(data) > 64*1024 {
				return HelperProductHandshake{}, errors.New("Dev Helper 握手回执超过安全大小限制")
			}
			var receipt helperHandshakeReceipt
			if err := json.Unmarshal(data, &receipt); err != nil {
				return HelperProductHandshake{}, errors.New("Dev Helper 握手回执不是有效 JSON")
			}
			handshake := receipt.Handshake
			if handshake.ContractVersion != HelperContractVersion || handshake.TaskID != prepared.TaskID ||
				handshake.ProductID != HelperProductID || handshake.ProductVersion != HelperProductVersion ||
				handshake.RegistrySchema != brain2.ProjectRegistrySchemaVersion ||
				handshake.DeviceID != prepared.DeviceID || handshake.ProjectID != prepared.ProjectID {
				return HelperProductHandshake{}, errors.New("Dev Helper 握手身份与本次启动任务不一致")
			}
			if handshake.Status != "accepted" {
				code := "UNKNOWN_REJECTION"
				if handshake.ErrorCode != nil && strings.TrimSpace(*handshake.ErrorCode) != "" {
					code = strings.TrimSpace(*handshake.ErrorCode)
				}
				return HelperProductHandshake{}, fmt.Errorf("Dev Helper 拒绝本次任务：%s", code)
			}
			if handshake.PathFingerprint != prepared.PathFingerprint || !sameScopes(handshake.AcceptedScopes, prepared.RequestedScopes) {
				return HelperProductHandshake{}, errors.New("Dev Helper 握手未严格确认本次授权范围")
			}
			return handshake, nil
		}
		if !os.IsNotExist(err) {
			return HelperProductHandshake{}, fmt.Errorf("读取 Dev Helper 握手回执失败: %w", err)
		}
		if time.Now().After(deadline) {
			return HelperProductHandshake{}, errors.New("等待 Dev Helper 握手超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func PrepareHelperLaunch(dataRoot, workspaceID, productRoot, localDeviceID, localMachineName string, project brain2.Project, now time.Time) (PreparedHelperLaunch, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return PreparedHelperLaunch{}, errors.New("Brain2 workspace ID 缺失")
	}
	if project.Status != "active" || project.PathStatus != "verified" || project.TruthStatus != "user_confirmed" {
		return PreparedHelperLaunch{}, errors.New("只有用户确认且路径已验证的项目可以授权 Dev Helper")
	}
	localDeviceID = strings.TrimSpace(localDeviceID)
	if localDeviceID == "" {
		return PreparedHelperLaunch{}, errors.New("本机 Brain2 设备 ID 缺失")
	}
	asset, canonical, err := chooseAsset(project, localDeviceID, localMachineName)
	if err != nil {
		return PreparedHelperLaunch{}, err
	}
	command, err := helperCommand(productRoot)
	if err != nil {
		return PreparedHelperLaunch{}, err
	}
	dataRoot = filepath.Clean(strings.TrimSpace(dataRoot))
	if dataRoot == "." || !filepath.IsAbs(dataRoot) {
		return PreparedHelperLaunch{}, errors.New("Launcher 数据目录必须是绝对路径")
	}
	taskID, err := randomID("hlt_")
	if err != nil {
		return PreparedHelperLaunch{}, err
	}
	nonce, err := randomID("")
	if err != nil {
		return PreparedHelperLaunch{}, err
	}
	grantID, err := randomID("hgrant_")
	if err != nil {
		return PreparedHelperLaunch{}, err
	}
	issuedAt := now.UTC()
	taskExpires := issuedAt.Add(5 * time.Minute)
	grantExpires := issuedAt.Add(10 * time.Minute)
	scopes := append([]string(nil), helperScopes...)
	task := HelperLaunchTask{
		ContractVersion: HelperContractVersion,
		TaskID:          taskID,
		Nonce:           nonce,
		IssuedAt:        issuedAt.Format(time.RFC3339Nano),
		ExpiresAt:       taskExpires.Format(time.RFC3339Nano),
		ProductID:       HelperProductID,
		ProductVersion:  HelperProductVersion,
		RequestedScopes: scopes,
		ProjectRef:      buildProjectRef(workspaceID, localDeviceID, project, asset, canonical),
		ProductGrant: HelperProductGrant{
			GrantVersion: HelperContractVersion,
			GrantID:      grantID,
			ProductID:    HelperProductID,
			ProjectID:    project.ID,
			AssetIDs:     []string{asset.ID},
			Scopes:       scopes,
			DeviceID:     localDeviceID,
			GrantedAt:    issuedAt.Format(time.RFC3339Nano),
			ExpiresAt:    grantExpires.Format(time.RFC3339Nano),
			RevokedAt:    nil,
			Activation:   "launcher-user-confirmed",
		},
	}
	launchRoot := filepath.Join(dataRoot, "runtime", "product-launches", taskID)
	taskPath := filepath.Join(launchRoot, "launch-task.json")
	receiptPath := filepath.Join(launchRoot, "product-handshake.json")
	if err := writeJSON(taskPath, task); err != nil {
		return PreparedHelperLaunch{}, fmt.Errorf("写入 Dev Helper 启动任务失败: %w", err)
	}
	return PreparedHelperLaunch{
		TaskID:          taskID,
		ProjectID:       project.ID,
		DeviceID:        localDeviceID,
		TaskPath:        taskPath,
		ReceiptPath:     receiptPath,
		PathFingerprint: task.ProjectRef.PathFingerprint,
		RequestedScopes: append([]string(nil), scopes...),
		Command:         command,
		Args:            []string{filepath.Clean(productRoot), "--launch-task", taskPath, "--launch-receipt", receiptPath},
		ExpiresAt:       taskExpires.Format(time.RFC3339Nano),
	}, nil
}

func sameScopes(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}
