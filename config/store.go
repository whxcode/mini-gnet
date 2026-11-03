package config

import "sync"

// Store 保存应用唯一的运行配置，允许主循环更新、后台任务读取。
type Store struct {
	mu    sync.RWMutex // 保护配置读写，不覆盖模型请求的耗时过程。
	value Config       // 当前应用配置。
}

// NewStore 创建共享配置对象；会话和任务不另外创建配置实例。
func NewStore(cfg Config) *Store {
	return &Store{value: cfg}
}

// Current 返回当前配置值，执行中的一轮对话使用这个局部值保持参数稳定。
func (s *Store) Current() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.value
}

// Set 更新共享配置，尚未开始执行的任务会读取新值。
func (s *Store) Set(cfg Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.value = cfg
}
