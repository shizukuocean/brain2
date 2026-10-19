package brain2

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"meow-launcher/internal/productregistry"
)

const MCPHostVersion = "0.6.3"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type MCPServer struct {
	fixedRoot     string
	registryPath  string
	reads         map[string][]string
	instanceID    string
	startedAt     string
	workspaceRoot map[string]string
}

func ServeMCP(root string, input io.Reader, output io.Writer) error {
	root, err := requireWorkspace(root)
	if err != nil {
		return err
	}
	server, err := newMCPServer(root, "")
	if err != nil {
		return err
	}
	return server.serve(input, output)
}

func ServeMCPRegistry(registryPath string, input io.Reader, output io.Writer) error {
	registryPath = filepath.Clean(strings.TrimSpace(registryPath))
	if registryPath == "." || !filepath.IsAbs(registryPath) {
		return errors.New("Brain2 MCP registry path must be absolute")
	}
	registry, err := productregistry.LoadFrom(registryPath)
	if err != nil {
		return fmt.Errorf("load Brain2 registry: %w", err)
	}
	if len(registry.Products) == 0 {
		return errors.New("Brain2 registry contains no workspaces")
	}
	server, err := newMCPServer("", registryPath)
	if err != nil {
		return err
	}
	return server.serve(input, output)
}

func newMCPServer(root, registryPath string) (*MCPServer, error) {
	instanceID, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	return &MCPServer{
		fixedRoot: root, registryPath: registryPath, reads: map[string][]string{},
		instanceID: instanceID, startedAt: time.Now().UTC().Format(time.RFC3339Nano),
		workspaceRoot: map[string]string{},
	}, nil
}

func (s *MCPServer) serve(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var request mcpRequest
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			if encodeErr := encoder.Encode(mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "invalid JSON"}}); encodeErr != nil {
				return encodeErr
			}
			continue
		}
		if len(request.ID) == 0 {
			continue
		}
		result, callErr := s.handle(request)
		response := mcpResponse{JSONRPC: "2.0", ID: request.ID, Result: result}
		if callErr != nil {
			response.Result = nil
			response.Error = &mcpError{Code: -32000, Message: callErr.Error()}
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *MCPServer) handle(request mcpRequest) (any, error) {
	switch request.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "brain2-meow", "version": MCPHostVersion},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools()}, nil
	case "tools/call":
		var call mcpToolCall
		if err := json.Unmarshal(request.Params, &call); err != nil {
			return nil, errors.New("invalid tools/call params")
		}
		return s.callTool(call)
	default:
		return nil, fmt.Errorf("unsupported MCP method %s", request.Method)
	}
}

func (s *MCPServer) callTool(call mcpToolCall) (any, error) {
	var result any
	var err error
	switch call.Name {
	case "brain2_get_bootstrap":
		result, err = s.bootstrap(call.Arguments)
	case "brain2_read_file":
		result, err = s.readFile(call.Arguments)
	case "brain2_health":
		result, err = s.health(call.Arguments)
	case "brain2_complete_handshake":
		result, err = s.completeHandshake(call.Arguments)
	case "brain2_submit_project_candidates":
		result, err = s.submitProjectCandidates(call.Arguments)
	case "brain2_submit_project_archive":
		result, err = s.submitProjectArchive(call.Arguments)
	default:
		err = fmt.Errorf("unknown Brain2 tool %s", call.Name)
	}
	if err != nil {
		errorCode := "brain2_tool_error"
		message := err.Error()
		if strings.HasPrefix(message, "[") {
			if end := strings.Index(message, "]"); end > 1 {
				errorCode = message[1:end]
				message = strings.TrimSpace(message[end+1:])
			}
		}
		diagnostic := map[string]any{
			"error_code": errorCode, "message": message,
			"mcp_host_version": MCPHostVersion, "mcp_instance_id": s.instanceID,
			"mcp_started_at": s.startedAt, "registry_path": s.registryPath,
		}
		data, _ := json.MarshalIndent(diagnostic, "", "  ")
		return map[string]any{
			"isError":           true,
			"content":           []map[string]any{{"type": "text", "text": string(data)}},
			"structuredContent": diagnostic,
		}, nil
	}
	data, marshalErr := json.MarshalIndent(result, "", "  ")
	if marshalErr != nil {
		return nil, marshalErr
	}
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(data)}},
		"structuredContent": result,
	}, nil
}

func (s *MCPServer) submitProjectCandidates(args map[string]any) (ProjectCandidateRegistry, error) {
	root, err := s.resolveScanRoot(stringArg(args, "scan_task_id"))
	if err != nil {
		return ProjectCandidateRegistry{}, err
	}
	task, err := LoadProjectScanTask(root, stringArg(args, "scan_task_id"))
	if err != nil {
		return ProjectCandidateRegistry{}, err
	}
	if task.Nonce != stringArg(args, "scan_nonce") || task.AgentID != strings.ToLower(stringArg(args, "agent_id")) {
		return ProjectCandidateRegistry{}, errors.New("项目扫描任务参数校验失败")
	}
	rawCandidates, ok := args["candidates"]
	if !ok {
		return ProjectCandidateRegistry{}, errors.New("缺少项目候选列表")
	}
	data, err := json.Marshal(rawCandidates)
	if err != nil {
		return ProjectCandidateRegistry{}, errors.New("项目候选格式无效")
	}
	var candidates []ProjectCandidateInput
	if err := json.Unmarshal(data, &candidates); err != nil {
		return ProjectCandidateRegistry{}, errors.New("项目候选格式无效")
	}
	if len(candidates) > 100 {
		return ProjectCandidateRegistry{}, errors.New("项目候选数量不能超过 100")
	}
	return SubmitProjectCandidates(root, task, candidates)
}

func (s *MCPServer) submitProjectArchive(args map[string]any) (ProjectArchiveReceipt, error) {
	taskID := stringArg(args, "archive_task_id")
	root, err := s.resolveArchiveRoot(taskID)
	if err != nil {
		return ProjectArchiveReceipt{}, err
	}
	task, err := LoadProjectArchiveTask(root, taskID)
	if err != nil {
		return ProjectArchiveReceipt{}, err
	}
	if task.Nonce != stringArg(args, "archive_nonce") || task.AgentID != strings.ToLower(stringArg(args, "agent_id")) {
		return ProjectArchiveReceipt{}, errors.New("项目建档任务参数校验失败")
	}
	rawArchive, ok := args["archive"]
	if !ok {
		return ProjectArchiveReceipt{}, errors.New("缺少项目档案")
	}
	data, err := json.Marshal(rawArchive)
	if err != nil {
		return ProjectArchiveReceipt{}, errors.New("项目档案格式无效")
	}
	var archive ProjectArchiveInput
	if err := json.Unmarshal(data, &archive); err != nil {
		return ProjectArchiveReceipt{}, errors.New("项目档案格式无效")
	}
	return SubmitProjectArchive(root, task, archive)
}

func (s *MCPServer) bootstrap(args map[string]any) (map[string]any, error) {
	root, task, err := s.validateTaskArgs(args)
	if err != nil {
		return nil, err
	}
	required := workspaceBootstrapFiles(root)
	documents := map[string]string{}
	for _, rel := range required {
		content, readErr := readWorkspaceFile(root, rel)
		if readErr != nil {
			return nil, fmt.Errorf("读取 Brain2 固定入口 %s 失败: %w", rel, readErr)
		}
		documents[rel] = content
	}
	s.reads[task.ConnectionID] = uniqueStrings(append(s.reads[task.ConnectionID], required...))
	workspaceID, _ := workspaceInstanceID(root)
	s.workspaceRoot[task.ConnectionID] = root
	return map[string]any{
		"protocol_version": ConnectionProtocolVersion,
		"connection_id":    task.ConnectionID,
		"agent_id":         task.AgentID,
		"brain2_root":      root,
		"workspace_id":     workspaceID,
		"mcp_host_version": MCPHostVersion,
		"mcp_instance_id":  s.instanceID,
		"mcp_started_at":   s.startedAt,
		"work_intent":      task.WorkIntent,
		"project_id":       task.ProjectID,
		"project_name":     task.ProjectName,
		"project_goal":     task.ProjectGoal,
		"required_files":   required,
		"documents":        documents,
		"next":             "使用同一 connection_id 运行 brain2_health，然后调用 brain2_complete_handshake。",
	}, nil
}

func (s *MCPServer) readFile(args map[string]any) (map[string]any, error) {
	connectionID := stringArg(args, "connection_id")
	root, err := s.resolveConnectionRoot(connectionID)
	if err != nil {
		return nil, err
	}
	task, err := LoadConnectionTask(root, connectionID)
	if err != nil {
		return nil, err
	}
	if task.Status == string(ConnectionConnected) {
		// Connected sessions may continue to use the governed reader.
	}
	rel := filepath.ToSlash(strings.TrimSpace(stringArg(args, "path")))
	content, err := readWorkspaceFile(root, rel)
	if err != nil {
		return nil, err
	}
	s.reads[connectionID] = uniqueStrings(append(s.reads[connectionID], rel))
	workspaceID, _ := workspaceInstanceID(root)
	return map[string]any{"path": rel, "content": content, "brain2_root": root, "workspace_id": workspaceID}, nil
}

func (s *MCPServer) completeHandshake(args map[string]any) (ConnectionReceipt, error) {
	root, task, err := s.validateTaskArgs(args)
	if err != nil {
		return ConnectionReceipt{}, err
	}
	report, err := Check(root)
	if err != nil {
		return ConnectionReceipt{}, err
	}
	input := HandshakeInput{
		ConnectionID:    task.ConnectionID,
		Nonce:           stringArg(args, "nonce"),
		AgentID:         stringArg(args, "agent_id"),
		AgentVersion:    stringArg(args, "agent_version"),
		Surface:         stringArg(args, "surface"),
		ProtocolVersion: ConnectionProtocolVersion,
		ReadFiles:       s.reads[task.ConnectionID],
		HealthStatus:    report.OverallStatus,
	}
	return CompleteConnection(root, input)
}

func (s *MCPServer) validateTaskArgs(args map[string]any) (string, ConnectionTask, error) {
	connectionID := stringArg(args, "connection_id")
	root, err := s.resolveConnectionRoot(connectionID)
	if err != nil {
		return "", ConnectionTask{}, err
	}
	task, err := LoadConnectionTask(root, connectionID)
	if err != nil {
		return "", ConnectionTask{}, err
	}
	if task.Nonce != stringArg(args, "nonce") || task.AgentID != strings.ToLower(stringArg(args, "agent_id")) {
		return "", ConnectionTask{}, errors.New("[connection_identity_mismatch] 连接参数校验失败")
	}
	return root, task, nil
}

func (s *MCPServer) health(args map[string]any) (HealthReport, error) {
	connectionID := stringArg(args, "connection_id")
	root, err := s.resolveConnectionRoot(connectionID)
	if err != nil {
		return HealthReport{}, err
	}
	report, err := Check(root)
	if err != nil {
		return HealthReport{}, err
	}
	report.MCPHostVersion = MCPHostVersion
	report.MCPInstanceID = s.instanceID
	report.MCPStartedAt = s.startedAt
	return compactHealthReport(report, 24), nil
}

func (s *MCPServer) resolveConnectionRoot(connectionID string) (string, error) {
	if !validToken(connectionID) {
		return "", errors.New("[invalid_connection_id] 无效的 connection_id")
	}
	return s.resolveWorkspace(func(root string) bool {
		info, err := os.Stat(filepath.Join(root, ".brain2", "connections", connectionID, "task.json"))
		return err == nil && info.Mode().IsRegular()
	}, "connection_task_not_found", "连接任务不存在于已登记的记忆库；当前 MCP 实例可能尚未刷新或任务已过期")
}

func (s *MCPServer) resolveScanRoot(taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || strings.ContainsAny(taskID, `/\\`) {
		return "", errors.New("[invalid_scan_task_id] 无效的项目扫描任务")
	}
	return s.resolveWorkspace(func(root string) bool {
		info, err := os.Stat(filepath.Join(root, ".brain2", "runtime", "project-scans", taskID+".json"))
		return err == nil && info.Mode().IsRegular()
	}, "project_scan_task_not_found", "项目扫描任务不存在于已登记的记忆库")
}

func (s *MCPServer) resolveArchiveRoot(taskID string) (string, error) {
	if !validToken(taskID) {
		return "", errors.New("[invalid_archive_task_id] 无效的项目建档任务")
	}
	return s.resolveWorkspace(func(root string) bool {
		info, err := os.Stat(projectArchiveTaskPath(root, taskID))
		return err == nil && info.Mode().IsRegular()
	}, "project_archive_task_not_found", "项目建档任务不存在于已登记的记忆库")
}

func (s *MCPServer) resolveWorkspace(matches func(string) bool, code, message string) (string, error) {
	if s.fixedRoot != "" {
		if matches(s.fixedRoot) {
			return s.fixedRoot, nil
		}
		return "", fmt.Errorf("[%s] %s；MCP 实际根目录: %s", code, message, s.fixedRoot)
	}
	registry, err := productregistry.LoadFrom(s.registryPath)
	if err != nil {
		return "", fmt.Errorf("[registry_unavailable] 读取本机记忆库注册表失败: %w", err)
	}
	for _, product := range registry.Products {
		if product.ProductID != "brain2" || !product.Installed {
			continue
		}
		root := platformWorkspaceRoot(product.RootPath)
		if IsWorkspace(root) && matches(root) {
			return root, nil
		}
	}
	return "", fmt.Errorf("[%s] %s；registry=%s；已登记记忆库=%d；host=%s；instance=%s", code, message, s.registryPath, len(registry.Products), MCPHostVersion, s.instanceID)
}

func platformWorkspaceRoot(root string) string {
	root = filepath.Clean(strings.TrimSpace(root))
	if runtime.GOOS == "windows" || len(root) < 3 || root[1] != ':' {
		return root
	}
	drive := strings.ToLower(root[:1])
	rest := strings.TrimLeft(strings.ReplaceAll(root[2:], `\`, "/"), "/")
	return filepath.Clean("/mnt/" + drive + "/" + rest)
}

func workspaceInstanceID(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".brain2", "config.json"))
	if err != nil {
		return "", err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", err
	}
	return cfg.InstanceID, nil
}

func readWorkspaceFile(root, rel string) (string, error) {
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "." || filepath.IsAbs(rel) {
		return "", errors.New("文件路径必须是 Brain2 内的相对路径")
	}
	target := filepath.Join(root, rel)
	relCheck, err := filepath.Rel(root, target)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(os.PathSeparator)) {
		return "", errors.New("拒绝读取 Brain2 根目录之外的文件")
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("目标不是普通文件")
	}
	if info.Size() > 2*1024*1024 {
		return "", errors.New("单文件超过 2 MiB 读取上限")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func mcpTools() []map[string]any {
	identitySchema := map[string]any{
		"type":     "object",
		"required": []string{"connection_id", "nonce", "agent_id"},
		"properties": map[string]any{
			"connection_id": map[string]any{"type": "string"},
			"nonce":         map[string]any{"type": "string"},
			"agent_id":      map[string]any{"type": "string"},
		},
	}
	return []map[string]any{
		{"name": "brain2_get_bootstrap", "description": "Validate a Launcher connection task and return the governed Brain2 entry documents.", "inputSchema": identitySchema},
		{"name": "brain2_read_file", "description": "Read one governed file inside the active Brain2 root.", "inputSchema": map[string]any{
			"type": "object", "required": []string{"connection_id", "path"},
			"properties": map[string]any{"connection_id": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}},
		}},
		{"name": "brain2_health", "description": "Run deterministic checks for the workspace bound to a Launcher connection task.", "inputSchema": map[string]any{
			"type": "object", "required": []string{"connection_id"},
			"properties": map[string]any{"connection_id": map[string]any{"type": "string"}},
		}},
		{"name": "brain2_submit_project_candidates", "description": "Submit Agent-discovered project candidates for Launcher review; this never confirms or registers projects automatically.", "inputSchema": map[string]any{
			"type": "object", "required": []string{"scan_task_id", "scan_nonce", "agent_id", "candidates"},
			"properties": map[string]any{
				"scan_task_id": map[string]any{"type": "string"}, "scan_nonce": map[string]any{"type": "string"}, "agent_id": map[string]any{"type": "string"},
				"candidates": map[string]any{"type": "array", "maxItems": 100, "items": map[string]any{
					"type": "object", "required": []string{"name", "root_path", "confidence", "evidence", "detected_kind"},
					"properties": map[string]any{
						"name": map[string]any{"type": "string"}, "root_path": map[string]any{"type": "string"},
						"suggested_goal":               map[string]any{"type": "string", "maxLength": 300},
						"confidence":                   map[string]any{"type": "number", "minimum": 0, "maximum": 1},
						"evidence":                     map[string]any{"type": "array", "maxItems": 8, "items": map[string]any{"type": "string"}},
						"detected_kind":                map[string]any{"enum": []string{"code_project", "application", "product", "needs_verification"}},
						"suggested_consumers":          map[string]any{"type": "array", "maxItems": 4, "items": map[string]any{"type": "string"}},
						"shared_repository_project_id": map[string]any{"type": "string", "description": "Existing project_id that owns the same code repository; the candidate remains an independent product, not a child project."},
					},
				}},
			},
		}},
		{"name": "brain2_submit_project_archive", "description": "Submit one user-selected project archive. The host validates the exact candidate, writes the governed project manifest, and returns a verified receipt.", "inputSchema": map[string]any{
			"type": "object", "required": []string{"archive_task_id", "archive_nonce", "agent_id", "archive"},
			"properties": map[string]any{
				"archive_task_id": map[string]any{"type": "string"}, "archive_nonce": map[string]any{"type": "string"}, "agent_id": map[string]any{"type": "string"},
				"archive": map[string]any{
					"type": "object", "required": []string{"name", "goal", "entity_kind", "lifecycle_stage", "evidence"},
					"properties": map[string]any{
						"name": map[string]any{"type": "string"}, "goal": map[string]any{"type": "string", "maxLength": 500},
						"entity_kind":     map[string]any{"enum": []string{"code_project", "application", "product", "needs_verification"}},
						"lifecycle_stage": map[string]any{"enum": []string{"discovery", "active", "paused", "completed", "archived", "needs_verification"}},
						"brain2_entry":    map[string]any{"type": "string"}, "parent_project_id": map[string]any{"type": "string"},
						"shared_repository_project_id": map[string]any{"type": "string", "description": "Existing project_id for an explicitly shared repository. This is not a parent-child relation."},
						"repository_url":               map[string]any{"type": "string"}, "default_branch": map[string]any{"type": "string"},
						"additional_paths": map[string]any{"type": "array", "maxItems": 12, "items": map[string]any{
							"type": "object", "required": []string{"path", "device_id"},
							"properties": map[string]any{"path": map[string]any{"type": "string"}, "device_id": map[string]any{"type": "string"}, "role": map[string]any{"type": "string"}, "status": map[string]any{"type": "string"}},
						}},
						"evidence":           map[string]any{"type": "array", "maxItems": 12, "items": map[string]any{"type": "string"}},
						"needs_verification": map[string]any{"type": "array", "maxItems": 12, "items": map[string]any{"type": "string"}},
					},
				},
			},
		}},
		{"name": "brain2_complete_handshake", "description": "Complete a validated Brain2 handshake and write the structured receipt.", "inputSchema": map[string]any{
			"type": "object", "required": []string{"connection_id", "nonce", "agent_id", "agent_version", "surface"},
			"properties": map[string]any{
				"connection_id": map[string]any{"type": "string"}, "nonce": map[string]any{"type": "string"},
				"agent_id": map[string]any{"type": "string"}, "agent_version": map[string]any{"type": "string"},
				"surface": map[string]any{"type": "string"},
			},
		}},
	}
}
