package command

import (
	menus "github.com/whxcode/mini-gnet/views/commands"
	"github.com/whxcode/mini-gnet/views/contextusage"
)

// handleContext 只读取当前会话快照，不请求模型、不将统计写入聊天历史。
func handleContext(a Application, args []string) error {
	state := a.ActiveSession().Snapshot()
	text := contextusage.Detail(state.ID, state.Title, state.Status, state.Usage)

	return menus.ShowContext(text)
}
