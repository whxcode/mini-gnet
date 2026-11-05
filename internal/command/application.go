package command

import (
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/mcp"
	"github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/views/message"
)

// Application 列出命令需要的应用操作。*app.App 实现它，命令包无需反向导入 app。
// Handler 仍然接收原来的 App 对象并直接执行操作，没有回调注册或待处理结果。
// 这些操作由主输入循环调用，不供后台 Agent 修改应用状态。
type Application interface {
	MCPManager() *mcp.Manager                           // 支持任务读取快照的共享服务管理器。
	ToolInfos() []tools.Info                            // 当前注册工具的定义与本地启动说明。
	ConfigStore() *config.Store                         // 读取和更新共享配置。
	ActiveSession() *session.Session                    // 获取当前会话。
	OpenSessions() []*session.Session                   // 按界面编号获取已打开会话。
	AttachSession(record *message.TempMessage)          // 接管记录、创建会话并启动队列。
	ActivateSession(s *session.Session)                 // 切换当前查看的会话。
	FindSession(value string) (*session.Session, error) // 按编号或 ID 查找。
	OpenedHistory(dir, id string) *session.Session      // 避免重复打开同一历史文件。
	RunningCount() int                                  // 统计执行中或等待中的会话。
	Notify(text string)                                 // 向当前会话写入提示。
	RequestExit()                                       // 通知主循环退出。
}
