// Package skill 发现工作目录中的技能，并读取可供模型选择的名称和简介。
package skill

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Summary 只保存技能摘要；正文、脚本和其他资源不会在扫描时注入模型。
type Summary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

// LoadDir 按目录名扫描直接子目录的 SKILL.md；目录不存在时返回空列表。
func LoadDir(directory string) ([]Summary, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("读取技能目录 %s: %w", directory, err)
	}

	var summaries []Summary
	names := make(map[string]string)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(directory, entry.Name(), "SKILL.md")
		data, err := readFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, err
		}

		document, err := parseDocument(data)
		if err != nil {
			return nil, fmt.Errorf("解析技能 %s: %w", path, err)
		}

		summary := document.Summary

		if previous, exists := names[summary.Name]; exists {
			return nil, fmt.Errorf("技能名称 %q 重复: %s 和 %s", summary.Name, previous, path)
		}

		summary.Path = path
		names[summary.Name] = path
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// parseDocument 分开解析 YAML 摘要与 Markdown 正文，支持引号和多行简介。
func parseDocument(data []byte) (Document, error) {
	text := strings.TrimPrefix(string(data), "\uFEFF")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	if strings.TrimSpace(lines[0]) != "---" {
		return Document{}, fmt.Errorf("SKILL.md 必须以 --- 开始的 YAML 元数据开头")
	}

	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
			break
		}
	}

	if end < 0 {
		return Document{}, fmt.Errorf("YAML 元数据缺少结束分隔符 ---")
	}

	// 用真正的 YAML 解析器读取字段；其他产品的扩展字段不参与本阶段的摘要展示。
	header := []byte(strings.Join(lines[1:end], "\n"))
	var metadata struct {
		Name        yaml.Node `yaml:"name"`
		Description yaml.Node `yaml:"description"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(header))
	if err := decoder.Decode(&metadata); err != nil {
		return Document{}, fmt.Errorf("YAML 元数据无效: %w", err)
	}

	if metadata.Name.Tag != "!!str" || metadata.Description.Tag != "!!str" {
		return Document{}, fmt.Errorf("name 和 description 必须是字符串")
	}

	summary := Summary{
		Name:        strings.TrimSpace(metadata.Name.Value),
		Description: strings.TrimSpace(metadata.Description.Value),
	}

	if summary.Name == "" || summary.Description == "" {
		return Document{}, fmt.Errorf("name 和 description 不能为空")
	}

	if strings.ContainsAny(summary.Name, "\r\n") {
		return Document{}, fmt.Errorf("name 必须是单行名称")
	}

	return Document{
		Summary: summary,
		Content: strings.TrimSpace(strings.Join(lines[end+1:], "\n")),
	}, nil
}

// readFile 统一限制技能文件大小，调用方通过 errors.Is 区分缺失文件与读取失败。
func readFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开技能 %s: %w", path, err)
	}

	data, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("读取技能 %s: %w", path, readErr)
	}

	if closeErr != nil {
		return nil, fmt.Errorf("关闭技能 %s: %w", path, closeErr)
	}

	if len(data) > 1<<20 {
		return nil, fmt.Errorf("技能文件 %s 超过 1 MiB", path)
	}

	return data, nil
}
