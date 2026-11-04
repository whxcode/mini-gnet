package skill

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Document 是已加载的技能说明；手动选择和工具调用共用同一种数据。
type Document struct {
	Summary
	Directory string `json:"directory"`
	Content   string `json:"content"`
}

// Discover 查找当前项目的技能摘要；空工作目录按进程当前目录解析。
func Discover(projectDir string) ([]Summary, error) {
	root, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录: %w", err)
	}

	return LoadDir(filepath.Join(root, ".agent", "skills"))
}

// Load 按已发现的 name 查找技能，不把模型给出的名称拼成任意文件路径。
func Load(projectDir, name string) (Document, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Document{}, fmt.Errorf("技能名称不能为空")
	}

	summaries, err := Discover(projectDir)
	if err != nil {
		return Document{}, err
	}

	for _, summary := range summaries {
		if summary.Name != name {
			continue
		}

		data, err := readFile(summary.Path)
		if err != nil {
			return Document{}, err
		}

		document, err := parseDocument(data)
		if err != nil {
			return Document{}, fmt.Errorf("解析技能 %s: %w", summary.Path, err)
		}

		if document.Name != name {
			return Document{}, fmt.Errorf("技能 %s 在加载时已更名，请重新选择", summary.Path)
		}

		if document.Content == "" {
			return Document{}, fmt.Errorf("技能 %s 没有正文", summary.Path)
		}

		document.Path = summary.Path
		document.Directory = filepath.Dir(summary.Path)

		return document, nil
	}

	return Document{}, fmt.Errorf("未找到技能 %q，输入 /skills 查看可用技能", name)
}
