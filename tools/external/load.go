// Package external 加载用户工具配置，并通过子进程调用 Python、Node.js 或其他可执行程序。
package external

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/whxcode/mini-gnet/tools"
)

// manifest 在公共工具定义之外声明启动方式；execution 不发送给模型。
type manifest struct {
	tools.Definition
	Execution execution `json:"execution"`
}

// execution 的 command 与 args 分开传给系统，不经过 Shell 字符串拼接。
type execution struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Cwd     string   `json:"cwd"`
}

// LoadDir 按目录名称顺序读取直接子目录的 tool.json；目录不存在时没有外部工具。
func LoadDir(directory string) ([]tools.Tool, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("读取工具目录 %s：%w", directory, err)
	}

	var implementations []tools.Tool

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(directory, entry.Name(), "tool.json")
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("打开工具定义 %s：%w", path, err)
		}

		// 每个定义最多 1 MiB；立即关闭文件，不累积到整个扫描结束。
		data, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取工具定义 %s：%w", path, readErr)
		}

		if len(data) > 1<<20 {
			return nil, fmt.Errorf("工具定义 %s 超过 1 MiB", path)
		}

		implementation, err := loadTool(path, data)
		if err != nil {
			return nil, fmt.Errorf("加载工具 %s：%w", path, err)
		}

		implementations = append(implementations, implementation)
	}

	return implementations, nil
}

// loadTool 校验配置并解析执行路径；此阶段不启动外部程序。
func loadTool(path string, data []byte) (*ProcessTool, error) {
	var config manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("tool.json 必须是单个 JSON 对象")
	}

	definitionJSON, err := json.Marshal(config.Definition)
	if err != nil {
		return nil, err
	}

	definition, err := tools.ParseDefinition(definitionJSON)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(config.Execution.Command) == "" {
		return nil, fmt.Errorf("execution.command 不能为空")
	}

	toolDirectory, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, err
	}

	directory := config.Execution.Cwd
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(toolDirectory, directory)
	}

	info, err := os.Stat(directory)
	if err != nil {
		return nil, fmt.Errorf("execution.cwd 无效：%w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("execution.cwd 必须是目录")
	}

	// 裸命令名由 PATH 查找；带路径的相对命令以 tool.json 所在目录为起点。
	command := config.Execution.Command
	if !filepath.IsAbs(command) && strings.ContainsAny(command, `/\`) {
		command = filepath.Join(toolDirectory, command)
	}

	return &ProcessTool{
		definition: definition,
		command:    command,
		args:       config.Execution.Args,
		cwd:        directory,
	}, nil
}
