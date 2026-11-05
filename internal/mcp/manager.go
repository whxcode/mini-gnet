package mcp

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// Manager 保存共享服务配置。后台任务读取快照，主循环串行修改，落盘后才更新内存。
// 当前不支持多个进程同时修改同一文件。
type Manager struct {
	mu      sync.RWMutex // 仅保护内存快照，不持锁等待文件或网络。
	writeMu sync.Mutex   // 串行化修改事务，读者仍可读取旧快照。
	path    string
	servers []Server
}

// Open 加载独立配置；文件不存在时返回空列表，损坏文件返回错误而不重建。
func Open(path string) (*Manager, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("MCP 配置文件路径不能为空")
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("解析 MCP 配置路径：%w", err)
	}

	servers, err := load(absolutePath)
	if err != nil {
		return nil, err
	}

	return &Manager{path: absolutePath, servers: servers}, nil
}

// Path 返回当前管理器实际加载的文件位置。
func (m *Manager) Path() string {
	return m.path
}

// List 返回列表副本，调用方编辑表单不会修改管理器的配置。
func (m *Manager) List() []Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Server(nil), m.servers...)
}

// Get 按唯一名称查找配置；名称区分大小写。
func (m *Manager) Get(name string) (Server, error) {
	for _, server := range m.List() {
		if server.Name == name {
			return server, nil
		}
	}

	return Server{}, fmt.Errorf("未配置 MCP 服务 %s", name)
}

// Search 在本地名称、简介和地址中搜索，不查询在线服务目录。
func (m *Manager) Search(keyword string) []Server {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	var matches []Server

	for _, server := range m.List() {
		text := server.Name + "\n" + server.Description + "\n" + server.URL
		if strings.Contains(strings.ToLower(text), keyword) {
			matches = append(matches, server)
		}
	}

	return matches
}

// Add 校验并保存新配置，重复名称不会覆盖原服务。
func (m *Manager) Add(server Server) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	server, err := Normalize(server)
	if err != nil {
		return err
	}

	for _, existing := range m.List() {
		if existing.Name == server.Name {
			return fmt.Errorf("MCP 服务 %s 已存在", server.Name)
		}
	}

	next := append(m.List(), server)
	if err := save(m.path, next); err != nil {
		return err
	}

	m.mu.Lock()
	m.servers = next
	m.mu.Unlock()
	return nil
}

// Update 替换指定服务，允许改名，但不能占用其他服务的名称。
func (m *Manager) Update(name string, server Server) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	server, err := Normalize(server)
	if err != nil {
		return err
	}

	index := -1
	for i, existing := range m.List() {
		if existing.Name == name {
			index = i
		} else if existing.Name == server.Name {
			return fmt.Errorf("MCP 服务 %s 已存在", server.Name)
		}
	}

	if index < 0 {
		return fmt.Errorf("未配置 MCP 服务 %s", name)
	}

	next := m.List()
	next[index] = server
	if err := save(m.path, next); err != nil {
		return err
	}

	m.mu.Lock()
	m.servers = next
	m.mu.Unlock()
	return nil
}

// Remove 先持久化删除结果，再移除内存中的配置。
func (m *Manager) Remove(name string) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	for i, server := range m.List() {
		if server.Name != name {
			continue
		}

		next := m.List()
		next = append(next[:i], next[i+1:]...)
		if err := save(m.path, next); err != nil {
			return err
		}

		m.mu.Lock()
		m.servers = next
		m.mu.Unlock()
		return nil
	}

	return fmt.Errorf("未配置 MCP 服务 %s", name)
}

// Select 一次读取所选服务的配置快照；排队任务开始时调用，不沿用上次任务的选择。
func (m *Manager) Select(names []string) ([]Server, error) {
	configured := m.List()
	var selected []Server
	for _, name := range names {
		found := false
		for _, server := range configured {
			if server.Name == name {
				selected = append(selected, server)
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("未配置 MCP 服务 %s", name)
		}
	}

	return selected, nil
}
