package agentconnect

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

var codexBrain2Table = regexp.MustCompile(`(?i)^\s*\[\s*mcp_servers\s*\.\s*(?:brain2-meow|"brain2-meow"|'brain2-meow')(?:\s*\.[^]]+)?\s*\]\s*(?:#.*)?$`)

type textRange struct {
	start int
	end   int
}

func codexConfigPath() (string, error) {
	root := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".codex")
	}
	return filepath.Join(root, "config.toml"), nil
}

func readCodexDesktopMCPRegistration() ([]byte, error) {
	path, err := codexConfigPath()
	if err != nil {
		return nil, err
	}
	config, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 Codex Desktop 配置失败: %w", err)
	}
	if err := validateCodexConfig(config); err != nil {
		return nil, err
	}
	return extractCodexBrain2Tables(config), nil
}

func registerCodexDesktopMCP(registryPath, hostCommand string, allowRegistryReplacement bool) error {
	path, err := codexConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建 Codex Desktop 配置目录失败: %w", err)
	}
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取 Codex Desktop 配置失败: %w", err)
	}
	if err := validateCodexConfig(current); err != nil {
		return err
	}
	existing := extractCodexBrain2Tables(current)
	if configUsesRegistry(existing) && !configContainsPath(existing, registryPath) && !allowRegistryReplacement {
		return errors.New("Codex Desktop 已有同名 Brain2 MCP 配置，但使用另一套 Launcher 注册表；未自动覆盖")
	}
	if configContainsPath(existing, registryPath) && configContainsExecutable(existing, hostCommand) {
		return nil
	}
	section := renderCodexBrain2Table(registryPath, hostCommand)
	updated := replaceCodexBrain2Tables(current, section)
	if err := validateCodexConfig(updated); err != nil {
		return fmt.Errorf("生成的 Codex Desktop 配置无效，未写入: %w", err)
	}
	latest, latestErr := os.ReadFile(path)
	if latestErr != nil && !os.IsNotExist(latestErr) {
		return fmt.Errorf("写入前复核 Codex Desktop 配置失败: %w", latestErr)
	}
	if !bytes.Equal(current, latest) {
		return errors.New("Codex Desktop 配置刚刚被其他程序修改，请关闭设置操作后重试")
	}
	if len(current) > 0 {
		backup := path + ".brain2-backup-" + time.Now().Format("20060102-150405.000")
		if err := os.WriteFile(backup, current, 0o600); err != nil {
			return fmt.Errorf("备份 Codex Desktop 配置失败，未执行更新: %w", err)
		}
	}
	if err := atomicWrite(path, updated); err != nil {
		return fmt.Errorf("写入 Codex Desktop 配置失败: %w", err)
	}
	return nil
}

func restoreCodexDesktopMCPRegistration(previous []byte) error {
	path, err := codexConfigPath()
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取 Codex Desktop 配置失败: %w", err)
	}
	if err := validateCodexConfig(current); err != nil {
		return err
	}
	updated := replaceCodexBrain2Tables(current, previous)
	if err := validateCodexConfig(updated); err != nil {
		return fmt.Errorf("回退后的 Codex Desktop 配置无效，未写入: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWrite(path, updated)
}

func validateCodexConfig(config []byte) error {
	if len(bytes.TrimSpace(config)) == 0 {
		return nil
	}
	var parsed map[string]any
	if err := toml.Unmarshal(config, &parsed); err != nil {
		return fmt.Errorf("Codex Desktop 的 config.toml 无法解析，未自动修改: %w", err)
	}
	return nil
}

func extractCodexBrain2Tables(config []byte) []byte {
	ranges := codexBrain2TableRanges(config)
	if len(ranges) == 0 {
		return nil
	}
	var output bytes.Buffer
	for _, item := range ranges {
		output.Write(config[item.start:item.end])
		if output.Len() > 0 && output.Bytes()[output.Len()-1] != '\n' {
			output.WriteByte('\n')
		}
	}
	return output.Bytes()
}

func replaceCodexBrain2Tables(config, replacement []byte) []byte {
	ranges := codexBrain2TableRanges(config)
	insertAt := len(config)
	if len(ranges) > 0 {
		insertAt = ranges[0].start
	}
	var output bytes.Buffer
	last := 0
	inserted := false
	for _, item := range ranges {
		output.Write(config[last:item.start])
		if !inserted {
			writeCodexSection(&output, replacement)
			inserted = true
		}
		last = item.end
	}
	if len(ranges) > 0 {
		output.Write(config[last:])
		return output.Bytes()
	}
	output.Write(config[:insertAt])
	writeCodexSection(&output, replacement)
	return output.Bytes()
}

func writeCodexSection(output *bytes.Buffer, section []byte) {
	section = bytes.TrimSpace(section)
	if len(section) == 0 {
		return
	}
	if output.Len() > 0 && output.Bytes()[output.Len()-1] != '\n' {
		output.WriteByte('\n')
	}
	if output.Len() > 0 && !bytes.HasSuffix(output.Bytes(), []byte("\n\n")) {
		output.WriteByte('\n')
	}
	output.Write(section)
	output.WriteByte('\n')
}

func codexBrain2TableRanges(config []byte) []textRange {
	lines := lineRanges(config)
	result := []textRange{}
	for index, line := range lines {
		value := strings.TrimSuffix(strings.TrimSuffix(string(config[line.start:line.end]), "\n"), "\r")
		if !codexBrain2Table.MatchString(value) {
			continue
		}
		end := len(config)
		for next := index + 1; next < len(lines); next++ {
			candidate := strings.TrimSpace(string(config[lines[next].start:lines[next].end]))
			if strings.HasPrefix(candidate, "[") {
				end = lines[next].start
				break
			}
		}
		result = append(result, textRange{start: line.start, end: end})
	}
	return mergeTextRanges(result)
}

func lineRanges(data []byte) []textRange {
	if len(data) == 0 {
		return nil
	}
	result := []textRange{}
	start := 0
	for index, value := range data {
		if value == '\n' {
			result = append(result, textRange{start: start, end: index + 1})
			start = index + 1
		}
	}
	if start < len(data) {
		result = append(result, textRange{start: start, end: len(data)})
	}
	return result
}

func mergeTextRanges(items []textRange) []textRange {
	if len(items) < 2 {
		return items
	}
	result := []textRange{items[0]}
	for _, item := range items[1:] {
		last := &result[len(result)-1]
		if item.start <= last.end {
			if item.end > last.end {
				last.end = item.end
			}
			continue
		}
		result = append(result, item)
	}
	return result
}

func renderCodexBrain2Table(registryPath, hostCommand string) []byte {
	return []byte("[mcp_servers.brain2-meow]\n" +
		"command = " + strconv.Quote(hostCommand) + "\n" +
		"args = [\"mcp-serve\", \"--registry\", " + strconv.Quote(registryPath) + "]\n")
}
