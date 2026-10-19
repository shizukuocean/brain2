package brain2

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type CheckResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	Path     string `json:"path"`
	Category string `json:"category,omitempty"`
}

type HealthReport struct {
	SchemaVersion   string                  `json:"schema_version"`
	GeneratedAt     string                  `json:"generated_at"`
	OverallStatus   string                  `json:"overall_status"`
	Score           int                     `json:"score"`
	Brain2Root      string                  `json:"brain2_root"`
	WorkspaceID     string                  `json:"workspace_id"`
	MCPHostVersion  string                  `json:"mcp_host_version,omitempty"`
	MCPInstanceID   string                  `json:"mcp_instance_id,omitempty"`
	MCPStartedAt    string                  `json:"mcp_started_at,omitempty"`
	FindingsTotal   int                     `json:"findings_total"`
	ReturnedChecks  int                     `json:"returned_checks"`
	ChecksTruncated bool                    `json:"checks_truncated"`
	Categories      []HealthCategorySummary `json:"categories"`
	Checks          []CheckResult           `json:"checks"`
}

type HealthCategorySummary struct {
	Category string `json:"category"`
	Status   string `json:"status"`
	OK       int    `json:"ok"`
	Warning  int    `json:"warning"`
	Error    int    `json:"error"`
	Info     int    `json:"info"`
}

func Check(root string) (HealthReport, error) {
	root, err := requireWorkspace(root)
	if err != nil {
		return HealthReport{}, err
	}
	required := append(workspaceBootstrapFiles(root),
		filepath.Join(".brain2", "config.json"),
		filepath.Join(".brain2", "projects.json"),
		filepath.Join(".brain2", "agents.json"),
		filepath.Join(".brain2", "version.json"),
	)
	agents, err := LoadAgents(root)
	if err != nil {
		return HealthReport{}, fmt.Errorf("读取 Agent 注册表失败: %w", err)
	}
	if len(agents.Agents) > 0 {
		for _, name := range []string{
			"AGENT_START_HERE.md", "FIRST_HANDSHAKE.md", "active-protocol.md",
			"current-adapter.md", "agent-config.json", "runtime-version.json",
		} {
			required = append(required, filepath.Join(".brain2", "runtime", name))
		}
	}

	workspaceID, _ := workspaceInstanceID(root)
	report := HealthReport{SchemaVersion: schemaVersion, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), OverallStatus: "healthy", Score: 100, Brain2Root: root, WorkspaceID: workspaceID, Checks: []CheckResult{}}
	for _, rel := range required {
		info, statErr := os.Stat(filepath.Join(root, rel))
		if statErr == nil && info.Mode().IsRegular() {
			report.Checks = append(report.Checks, CheckResult{Name: rel, Status: "ok", Message: "存在", Path: rel})
			continue
		}
		report.Checks = append(report.Checks, CheckResult{Name: rel, Status: "error", Message: "缺失或不是文件", Path: rel})
		report.Score -= 15
		report.OverallStatus = "critical"
	}
	projects, err := LoadProjects(root)
	if err != nil {
		return HealthReport{}, fmt.Errorf("读取项目注册表失败: %w", err)
	}
	for _, check := range projectManifestHealthChecks(root, projects) {
		report.Checks = append(report.Checks, check)
		switch check.Status {
		case "warning":
			report.Score -= 5
			if report.OverallStatus == "healthy" {
				report.OverallStatus = "needs_attention"
			}
		case "error":
			report.Score -= 10
			if report.OverallStatus == "healthy" {
				report.OverallStatus = "needs_attention"
			}
		}
	}
	graphChecks := knowledgeGraphHealthChecks(root)
	report.Checks = append(report.Checks, graphChecks...)
	graphNeedsAttention := false
	for _, check := range graphChecks {
		if check.Status == "warning" || check.Status == "error" {
			graphNeedsAttention = true
			break
		}
	}
	if graphNeedsAttention {
		report.Score -= 5
		if report.OverallStatus == "healthy" {
			report.OverallStatus = "needs_attention"
		}
	}
	if report.Score < 0 {
		report.Score = 0
	}
	finalizeHealthSummary(&report)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return report, err
	}
	if err := atomicWrite(filepath.Join(root, ".brain2", "health-report.json"), append(data, '\n'), 0o644); err != nil {
		return report, err
	}
	return report, nil
}

func finalizeHealthSummary(report *HealthReport) {
	order := []string{"connection_core", "project_registry", "content_graph", "report"}
	summaries := map[string]*HealthCategorySummary{}
	for _, category := range order {
		summaries[category] = &HealthCategorySummary{Category: category, Status: "healthy"}
	}
	for index := range report.Checks {
		check := &report.Checks[index]
		check.Category = healthCategory(*check)
		summary, ok := summaries[check.Category]
		if !ok {
			summary = &HealthCategorySummary{Category: check.Category, Status: "healthy"}
			summaries[check.Category] = summary
			order = append(order, check.Category)
		}
		switch check.Status {
		case "error":
			summary.Error++
			summary.Status = "needs_attention"
		case "warning":
			summary.Warning++
			if summary.Status == "healthy" {
				summary.Status = "needs_attention"
			}
		case "info":
			summary.Info++
		case "ok":
			summary.OK++
		}
	}
	report.Categories = make([]HealthCategorySummary, 0, len(order))
	for _, category := range order {
		summary := summaries[category]
		if summary.OK+summary.Warning+summary.Error+summary.Info > 0 {
			report.Categories = append(report.Categories, *summary)
		}
	}
	report.FindingsTotal = len(report.Checks)
	report.ReturnedChecks = len(report.Checks)
	report.ChecksTruncated = false
}

func healthCategory(check CheckResult) string {
	if check.Category != "" {
		return check.Category
	}
	if strings.HasPrefix(check.Name, "project_manifest:") || strings.HasPrefix(check.Name, "project_record:") {
		return "project_registry"
	}
	if strings.HasPrefix(filepath.ToSlash(check.Path), ".brain2/") || check.Path == "NAV.md" || strings.Contains(filepath.ToSlash(check.Path), "START_HERE.md") {
		return "connection_core"
	}
	return "content_graph"
}

func compactHealthReport(report HealthReport, limit int) HealthReport {
	if limit < 1 || len(report.Checks) <= limit {
		return report
	}
	selected := make([]CheckResult, 0, limit)
	for _, check := range report.Checks {
		if check.Status != "ok" {
			selected = append(selected, check)
			if len(selected) == limit {
				break
			}
		}
	}
	for _, check := range report.Checks {
		if len(selected) == limit {
			break
		}
		if check.Status == "ok" {
			selected = append(selected, check)
		}
	}
	report.Checks = selected
	report.ReturnedChecks = len(selected)
	report.ChecksTruncated = true
	return report
}

var (
	wikiLinkPattern     = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	markdownLinkPattern = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
)

type markdownDocument struct {
	rel     string
	key     string
	content string
}

func knowledgeGraphHealthChecks(root string) []CheckResult {
	documents := []markdownDocument{}
	byKey := map[string]string{}
	byBase := map[string][]string{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if path != root && shouldSkipHealthDirectory(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(info.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		key := markdownPathKey(rel)
		documents = append(documents, markdownDocument{rel: rel, key: key, content: string(data)})
		byKey[key] = rel
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)))
		byBase[base] = append(byBase[base], rel)
		return nil
	})
	sort.Slice(documents, func(i, j int) bool { return documents[i].rel < documents[j].rel })

	inbound := map[string]int{}
	checks := []CheckResult{}
	seenFindings := map[string]bool{}
	const maxFindings = 160
	appendFinding := func(check CheckResult) {
		key := check.Category + "\x00" + check.Name + "\x00" + check.Path
		if seenFindings[key] {
			return
		}
		seenFindings[key] = true
		if len(checks) < maxFindings {
			checks = append(checks, check)
		}
	}
	for _, doc := range documents {
		for _, match := range wikiLinkPattern.FindAllStringSubmatch(doc.content, -1) {
			target := cleanWikiTarget(match[1])
			if target == "" {
				continue
			}
			if ext := strings.ToLower(filepath.Ext(target)); ext != "" && ext != ".md" {
				rootTarget := filepath.Join(root, filepath.FromSlash(target))
				relativeTarget := filepath.Join(root, filepath.FromSlash(filepath.Dir(doc.rel)), filepath.FromSlash(target))
				if _, rootErr := os.Stat(rootTarget); rootErr != nil {
					if _, relativeErr := os.Stat(relativeTarget); relativeErr != nil {
						appendFinding(CheckResult{Name: "附件引用缺失：" + target, Status: "warning", Message: "Obsidian 附件目标不存在", Path: doc.rel, Category: "missing_attachment"})
					}
				}
				continue
			}
			resolved, ambiguous := resolveWikiTarget(doc.rel, target, byKey, byBase)
			if len(ambiguous) > 1 {
				appendFinding(CheckResult{Name: "短链歧义：" + target, Status: "warning", Message: "同名目标有多个候选：" + strings.Join(ambiguous, "；"), Path: doc.rel, Category: "ambiguous_link"})
				continue
			}
			if resolved == "" {
				appendFinding(CheckResult{Name: "断链：" + target, Status: "warning", Message: "Wiki 链接目标不存在", Path: doc.rel, Category: "broken_link"})
				continue
			}
			inbound[markdownPathKey(resolved)]++
		}
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(doc.content, -1) {
			target := cleanMarkdownTarget(match[1])
			if target == "" || isExternalTarget(target) {
				continue
			}
			resolved := filepath.Clean(filepath.Join(root, filepath.FromSlash(filepath.Dir(doc.rel)), filepath.FromSlash(target)))
			if _, err := os.Stat(resolved); err != nil {
				appendFinding(CheckResult{Name: "本地引用缺失：" + target, Status: "warning", Message: "Markdown 本地链接或附件目标不存在", Path: doc.rel, Category: "missing_local_target"})
			}
		}
	}

	for _, doc := range documents {
		if inbound[doc.key] != 0 || isExpectedEntryDocument(doc.rel) {
			continue
		}
		appendFinding(CheckResult{Name: "孤立候选：" + filepath.Base(doc.rel), Status: "info", Message: "当前扫描未发现其他 Markdown 文件指向它；这只是候选，不代表需要修改", Path: doc.rel, Category: "isolation_candidate"})
	}
	if len(checks) == maxFindings {
		checks = append(checks, CheckResult{Name: "检查结果已截断", Status: "info", Message: "候选项过多，报告仅保留前 160 项；请交给 Agent 分批分析", Path: ".brain2/health-report.json", Category: "report_limit"})
	}
	return checks
}

func shouldSkipHealthDirectory(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ".brain2", ".obsidian", ".git", ".trash", "node_modules":
		return true
	default:
		return false
	}
}

func markdownPathKey(path string) string {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	path = strings.TrimSuffix(path, filepath.Ext(path))
	return strings.ToLower(strings.TrimPrefix(path, "./"))
}

func cleanWikiTarget(value string) string {
	value = strings.TrimSpace(strings.SplitN(value, "|", 2)[0])
	value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
	value = strings.TrimSpace(strings.SplitN(value, "^", 2)[0])
	return strings.TrimPrefix(value, "/")
}

func resolveWikiTarget(sourceRel, target string, byKey map[string]string, byBase map[string][]string) (string, []string) {
	if ext := strings.ToLower(filepath.Ext(target)); ext != "" && ext != ".md" {
		return "", nil
	}
	if strings.ContainsAny(target, `/\\`) {
		rootKey := markdownPathKey(target)
		if resolved := byKey[rootKey]; resolved != "" {
			return resolved, nil
		}
		relative := filepath.Join(filepath.Dir(sourceRel), filepath.FromSlash(target))
		if resolved := byKey[markdownPathKey(relative)]; resolved != "" {
			return resolved, nil
		}
		return "", nil
	}
	base := strings.ToLower(strings.TrimSuffix(target, filepath.Ext(target)))
	candidates := byBase[base]
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return "", candidates
}

func cleanMarkdownTarget(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "<>"))
	if index := strings.Index(value, ` "`); index >= 0 {
		value = value[:index]
	}
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	return strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
}

func isExternalTarget(target string) bool {
	lower := strings.ToLower(target)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "obsidian:") || strings.HasPrefix(lower, "data:")
}

func isExpectedEntryDocument(rel string) bool {
	base := strings.ToLower(filepath.Base(rel))
	return base == "nav.md" || base == "readme.md" || base == "start_here.md" || strings.Contains(base, "索引") || strings.Contains(base, "当前上下文")
}

func projectManifestHealthChecks(root string, registry ProjectRegistry) []CheckResult {
	checks := []CheckResult{}
	for _, project := range registry.Projects {
		manifestPath := strings.TrimSpace(project.ManifestPath)
		if manifestPath == "" {
			if hasProjectCapability(project, machineProjectCapability) {
				checks = append(checks, machineProjectRecordHealthCheck(root, project))
				continue
			}
			checks = append(checks, CheckResult{
				Name: "project_manifest:" + project.ID, Status: "warning",
				Message: "旧项目尚未建立项目身份证；不会自动猜测路径，请在用户确认后补登记",
				Path:    project.Brain2Entry,
			})
			continue
		}
		content, err := readWorkspaceFile(root, manifestPath)
		if err != nil {
			checks = append(checks, CheckResult{
				Name: "project_manifest:" + project.ID, Status: "error",
				Message: "项目机器索引已登记身份证，但文件缺失或不可读",
				Path:    manifestPath,
			})
			continue
		}
		values := readManifestScalars(content)
		mismatches := []string{}
		identityFields := []struct{ key, want string }{
			{"brain2_project_manifest", project.ManifestVersion},
			{"project_id", project.ID},
			{"project_name", project.Name},
			{"project_mode", project.Mode},
			{"project_status", project.Status},
			{"asset_status", project.AssetStatus},
			{"path_status", project.PathStatus},
			{"truth_status", project.TruthStatus},
		}
		if project.SharedRepositoryProjectID != "" {
			identityFields = append(identityFields, struct{ key, want string }{"shared_repository_project_id", project.SharedRepositoryProjectID})
			if !projectIDExists(registry, project.SharedRepositoryProjectID) {
				mismatches = append(mismatches, "shared_repository_project_id:missing")
			}
		}
		for _, field := range identityFields {
			if values[field.key] != field.want {
				mismatches = append(mismatches, field.key)
			}
		}
		if len(mismatches) > 0 {
			checks = append(checks, CheckResult{
				Name: "project_manifest:" + project.ID, Status: "error",
				Message: "项目身份证与机器索引不一致：" + strings.Join(mismatches, ", "),
				Path:    manifestPath,
			})
			continue
		}
		invalidAssets := []string{}
		for _, asset := range project.Assets {
			if asset.Status != "verified" || asset.Path == "" {
				continue
			}
			assetPath := asset.Path
			if asset.PathKind == "brain2_relative" {
				assetPath = filepath.Join(root, filepath.FromSlash(asset.Path))
			}
			if asset.PathKind == "absolute" || asset.PathKind == "brain2_relative" {
				if _, statErr := os.Stat(assetPath); statErr != nil {
					invalidAssets = append(invalidAssets, asset.ID)
				}
			}
		}
		if len(invalidAssets) > 0 {
			checks = append(checks, CheckResult{
				Name: "project_manifest:" + project.ID, Status: "warning",
				Message: "已验证路径当前不可访问：" + strings.Join(invalidAssets, ", "),
				Path:    manifestPath,
			})
			continue
		}
		checks = append(checks, CheckResult{
			Name: "project_manifest:" + project.ID, Status: "ok",
			Message: "项目身份证、机器索引与当前路径一致",
			Path:    manifestPath,
		})
	}
	return checks
}

func machineProjectRecordHealthCheck(root string, project Project) CheckResult {
	result := CheckResult{Name: "project_record:" + project.ID, Path: project.Brain2Entry}
	entryPath, err := governedProjectEntryPath(root, project.Brain2Entry)
	if err != nil {
		result.Status = "error"
		result.Message = "Launcher 项目记录路径无效"
		return result
	}
	data, err := os.ReadFile(entryPath)
	if err != nil {
		result.Status = "error"
		result.Message = "Launcher 项目记录丢失或不可读"
		return result
	}
	var record MachineProjectRecord
	if json.Unmarshal(data, &record) != nil || record.Project.ID != project.ID || record.Project.Name != project.Name {
		result.Status = "error"
		result.Message = "Launcher 项目记录与项目索引不一致"
		return result
	}
	if project.SourcePath == nil {
		result.Status = "warning"
		result.Message = "项目已登记，但缺少本机工程路径"
		return result
	}
	if info, statErr := os.Stat(*project.SourcePath); statErr != nil || !info.IsDir() {
		result.Status = "warning"
		result.Message = "项目已登记，但本机工程路径当前不可访问"
		return result
	}
	result.Status = "ok"
	result.Message = "项目机器记录、用户确认状态与本机路径一致"
	return result
}
