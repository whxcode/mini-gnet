package conversation

import (
	"context"
	"os"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

type finished struct{}

// Run 消费应用提供的消息通道，只负责显示和取消，不创建或执行 Agent。
// 通道关闭表示 Agent 已发送完本轮输出；业务错误由应用获取并报告。
func Run(ctx context.Context, cancel context.CancelFunc, events <-chan Event) error {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		for event := range events {
			Print(event)
		}
		return nil
	}
	result, err := tea.NewProgram(newStream(ctx, cancel, events), tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout)).Run()
	if err != nil {
		return err
	}
	return result.(streamModel).err
}

type streamModel struct {
	ctx    context.Context
	cancel context.CancelFunc
	events <-chan Event
	spin   spinner.Model
	width  int
	status Event
	live   string
	done   bool
	err    error
}

// newStream 初始化预览状态，只持有消息的读取端。
func newStream(ctx context.Context, cancel context.CancelFunc, events <-chan Event) streamModel {
	s := spinner.New(spinner.WithSpinner(spinner.Dot))
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	return streamModel{
		ctx: ctx, cancel: cancel, events: events,
		spin: s, width: 80, status: Event{Role: Assistant, Text: "正在等待模型…"},
	}
}

// Init 开始读取消息和刷新加载动画；网络与工具已经由应用独立启动。
func (m streamModel) Init() tea.Cmd {
	return tea.Batch(m.nextEvent(), m.spin.Tick)
}

// nextEvent 将通道读取适配为 Bubble Tea 的 Cmd；这是框架要求的界面适配代码。
func (m streamModel) nextEvent() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-m.events
		if !ok {
			return finished{}
		}
		return event
	}
}

// Update 按事件更新预览；Ctrl+C 只请求取消，等通道关闭后再退出。
func (m streamModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(4, msg.Width-1)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			// 等工作协程退出后再恢复输入，避免上一轮仍写会话时用户又开始下一轮。
			m.cancel()
			m.status = Event{Role: System, Text: "正在停止当前任务…"}
		}
	case Event:
		switch msg.Kind {
		case Status:
			m.status = msg
			m.live = ""
		case Delta:
			m.live += msg.Text
		case Message:
			if msg.Role == Assistant {
				m.live = ""
			}
			// 先打印完整消息，再读取下一事件，防止快速输出时消息顺序颠倒。
			return m, tea.Sequence(tea.Println(Format(msg, m.width)), m.nextEvent())
		}
		return m, m.nextEvent()
	case finished:
		m.done = true
		if m.ctx.Err() != nil {
			m.err = m.ctx.Err()
		}
		if m.live != "" {
			// 请求中断时，保留已经收到的文字，但明确标注它不是完整回答。
			partial := Event{Role: Assistant, Name: "未完成", Text: m.live}
			return m, tea.Sequence(tea.Println(Format(partial, m.width)), tea.Quit)
		}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

// preview 只截取最后两行“显示行”，长中文行也先按终端宽度换行。
// 完整内容始终由 Agent 保留，预览截断不会影响最终回答或会话存储。
func preview(s string, width int) string {
	lines := strings.Split(ansi.Hardwrap(cleanText(s), max(1, width), true), "\n")
	if len(lines) > 2 {
		lines = lines[len(lines)-2:]
	}
	for len(lines) < 2 {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// View 绘制状态和最新两行输出，完成后清空预览区域。
func (m streamModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	title := m.spin.View() + " " + Header(m.status.Role, m.status.Name) + "  " + cleanText(m.status.Text)
	title = ansi.Truncate(title, m.width, "…")
	style, _ := roleStyle(Assistant)
	body := preview(m.live, max(1, m.width-2))
	return tea.NewView(title + "\n" + style.Render("│ "+strings.ReplaceAll(body, "\n", "\n│ ")))
}
