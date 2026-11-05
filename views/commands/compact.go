package commands

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type compactDoneMsg struct{}

// compactModel 只等待命令层关闭 done，模型结果仍由命令层保存和处理。
type compactModel struct {
	ctx      context.Context
	done     <-chan struct{}
	width    int
	canceled bool
}

// WaitCompact 显示请求中的状态，Esc 只返回取消意图；创建方负责取消并等待后台请求退出。
func WaitCompact(parent context.Context, done <-chan struct{}) (bool, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel() // 菜单提前退出时，解除 Init 对 done 的等待。

	model, err := tea.NewProgram(compactModel{
		ctx:   ctx,
		done:  done,
		width: 78,
	}).Run()
	if err != nil {
		return false, err
	}
	return model.(compactModel).canceled, nil
}

// Init 等待完成或上游取消，不发起模型请求。
func (m compactModel) Init() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.done:
		case <-m.ctx.Done():
		}
		return compactDoneMsg{}
	}
}

// Update 处理退出与窗口尺寸；完成后回到命令层决定是否创建会话。
func (m compactModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case compactDoneMsg:
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width = max(8, msg.Width-1)
	case tea.KeyPressMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" {
			m.canceled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View 展示简短等待说明，中文按终端列宽折行。
func (m compactModel) View() tea.View {
	lines := []string{
		accent.Render("正在总结当前会话…"),
		"成功后创建摘要续聊会话，原会话保留。",
		muted.Render("Esc 取消 · 最长等待 3 分钟"),
	}
	text := ansi.Hardwrap(strings.Join(lines, "\n"), max(1, m.width-2), true)
	return tea.NewView(panel.Width(m.width).Render(text))
}
