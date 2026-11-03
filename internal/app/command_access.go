package app

import (
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/command"
	"github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/tools"
)

// 编译时确认 App 满足命令所需的操作，不需要运行时注册回调。
var _ command.Application = (*App)(nil)

// ConfigStore 返回应用唯一的共享配置，命令保存后直接更新它。
func (a *App) ConfigStore() *config.Store {
	return a.config
}

// ActiveSession 返回当前接收输入和展示结果的会话。
func (a *App) ActiveSession() *session.Session {
	return a.CurrentSession
}

// OpenSessions 返回按编号排列的会话列表副本，调用者不能修改应用列表。
func (a *App) OpenSessions() []*session.Session {
	return append([]*session.Session(nil), a.opened...)
}

// RequestExit 标记退出，资源释放仍由主入口统一调用 Close 完成。
func (a *App) RequestExit() {
	a.quit = true
}

// ToolDefinitions 从共享注册表读取工具，/tools 与发送给模型的声明来自同一份数据。
func (a *App) ToolDefinitions() []tools.Definition {
	return a.registry.Definitions()
}
