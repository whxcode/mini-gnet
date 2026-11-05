package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// document 带版本保存配置，为以后迁移格式保留明确边界。
type document struct {
	Version int      `json:"version"`
	Servers []Server `json:"servers"`
}

// load 校验整个文件，拒绝损坏内容、未知版本和重复名称。
func load(path string) ([]Server, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("读取 MCP 配置 %s：%w", path, err)
	}

	var doc document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields() // 不支持的字段应报错，避免下一次保存时静默丢失。
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("解析 MCP 配置 %s：%w", path, err)
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("MCP 配置 %s：文件末尾包含多余或无效内容", path)
	}

	if doc.Version != 1 {
		return nil, fmt.Errorf("MCP 配置 %s：不支持版本 %d", path, doc.Version)
	}

	names := make(map[string]bool)
	for i, server := range doc.Servers {
		normalized, err := Normalize(server)
		if err != nil {
			return nil, fmt.Errorf("MCP 配置 %s：%w", path, err)
		}

		if names[normalized.Name] {
			return nil, fmt.Errorf("MCP 配置 %s：服务 %s 重复", path, normalized.Name)
		}

		names[normalized.Name] = true
		doc.Servers[i] = normalized
	}

	return doc.Servers, nil
}

// save 使用同目录临时文件替换旧配置，写入失败时保留原文件。
func save(path string, servers []Server) error {
	if servers == nil {
		servers = []Server{}
	}

	data, err := json.MarshalIndent(document{Version: 1, Servers: servers}, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 MCP 配置：%w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建 MCP 配置目录：%w", err)
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".mcp-*.json")
	if err != nil {
		return fmt.Errorf("创建 MCP 临时文件：%w", err)
	}

	defer os.Remove(file.Name()) // 替换成功后临时文件已经不存在。

	_, writeErr := file.Write(append(data, '\n'))
	if writeErr == nil {
		writeErr = file.Sync()
	}

	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("写入 MCP 配置 %s：%w", path, writeErr)
	}

	if closeErr != nil {
		return fmt.Errorf("关闭 MCP 配置 %s：%w", path, closeErr)
	}

	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("替换 MCP 配置 %s：%w", path, err)
	}

	return nil
}
