package commands

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/internal/mcp"
)

// MCPAction 是菜单交给命令层的操作；界面不读写配置文件。
type MCPAction struct {
	Kind string
	Name string
}

// ManageMCP 展示服务列表，返回一次明确操作；空操作表示 Esc 返回。
func ManageMCP(servers []mcp.Server, path, notice string) (MCPAction, error) {
	model := newMCP(servers, path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model.ctx = ctx
	model.notice = notice
	result, err := tea.NewProgram(model).Run()
	if err != nil {
		return MCPAction{}, err
	}

	return result.(mcpModel).action, nil
}

// MCPText 生成本地配置详情，明确区分已配置与已连接。
func MCPText(servers []mcp.Server, path string) string {
	lines := []string{"MCP 配置文件：" + path, "仅本地管理 · 未建立连接", ""}
	if len(servers) == 0 {
		lines = append(lines, "没有匹配的服务。使用 /mcp add 添加。")
	}

	for _, server := range servers {
		lines = append(lines,
			"◆ "+server.Name+" · 已配置 · 未连接",
			"简介："+server.Description,
			"传输："+server.Transport,
			"地址："+server.URL,
			"",
		)
	}

	return strings.Join(lines, "\n")
}

// mcpModel 管理列表、搜索和详情；删除需在确认页再次按 Enter。
type mcpModel struct {
	ctx            context.Context // 菜单生命周期，退出后取消未完成请求。
	requestContext context.Context
	cancel         context.CancelFunc
	generation     int // 忽略退出详情后到达的旧响应。
	loading        bool
	remote         *mcp.Details
	remoteError    string
	servers        []mcp.Server
	path           string
	filter         textinput.Model
	searching      bool
	cursor         int
	width          int
	height         int
	detail         bool
	detailOnly     bool
	confirm        bool
	offset         int
	notice         string
	action         MCPAction
	done           bool
}

// newMCP 初始化只读列表副本和本地搜索输入框。
func newMCP(servers []mcp.Server, path string) mcpModel {
	filter := textinput.New()
	filter.Prompt = "搜索："
	filter.SetVirtualCursor(true)

	return mcpModel{
		ctx:     context.Background(),
		servers: append([]mcp.Server(nil), servers...),
		path:    path,
		filter:  filter,
		width:   78,
		height:  24,
	}
}

// Init 列表直接显示；从命令直接打开详情时启动一次远程发现。
func (m mcpModel) Init() tea.Cmd {
	if m.loading {
		return m.requestDetails()
	}
	return nil
}

// matches 过滤当前本地列表，输入搜索词不会产生网络请求。
func (m mcpModel) matches() []mcp.Server {
	keyword := strings.ToLower(m.filter.Value())
	var result []mcp.Server
	for _, server := range m.servers {
		if strings.Contains(strings.ToLower(server.Name+"\n"+server.Description+"\n"+server.URL), keyword) {
			result = append(result, server)
		}
	}

	return result
}

// Update 按页面处理键盘事件；Esc 从详情或搜索退回列表，再返回聊天。
func (m mcpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(mcpDetailsMsg); ok {
		if !m.detail || result.generation != m.generation {
			return m, nil
		}
		m.loading = false
		m.remote = result.details
		m.remoteError = ""
		if result.err != nil {
			m.remoteError = result.err.Error()
		}
		return m, nil
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = max(8, size.Width-1)
		m.height = max(5, size.Height)
		position := m.filter.Position()
		m.filter.SetWidth(max(1, m.width-9))
		m.filter.CursorEnd()
		m.filter.SetCursor(position)
		return m, nil
	}

	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if m.searching {
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.cursor = 0
			return m, cmd
		}

		return m, nil
	}

	if key.String() == "ctrl+c" {
		if m.cancel != nil {
			m.cancel()
		}
		m.done = true
		return m, tea.Quit
	}

	if key.String() == "esc" {
		switch {
		case m.confirm:
			m.confirm = false
		case m.detail:
			if m.cancel != nil {
				m.cancel()
			}
			if m.detailOnly {
				m.done = true
				return m, tea.Quit
			}

			m.detail = false
			m.offset = 0
		case m.searching:
			m.searching = false
			m.filter.Blur()
		default:
			m.done = true
			return m, tea.Quit
		}

		return m, nil
	}

	if m.searching {
		if key.String() == "enter" {
			m.searching = false
			m.filter.Blur()
			return m, nil
		}

		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.cursor = 0
		return m, cmd
	}

	items := m.matches()
	if m.confirm {
		if key.String() == "enter" && len(items) > 0 {
			m.action = MCPAction{Kind: "remove", Name: items[m.cursor].Name}
			m.done = true
			return m, tea.Quit
		}

		return m, nil
	}

	if m.detail {
		switch key.String() {
		case "r":
			return m, m.beginDetails()
		case "down":
			m.offset++
		case "up":
			m.offset--
		case "pgdown":
			m.offset += max(1, m.height-2)
		case "pgup":
			m.offset -= max(1, m.height-2)
		}

		m.offset = min(max(0, m.offset), max(0, len(m.detailLines())-max(1, m.height-2)))
		return m, nil
	}

	switch key.String() {
	case "/":
		m.searching = true
		return m, m.filter.Focus()
	case "up":
		m.cursor = max(0, m.cursor-1)
	case "down":
		m.cursor = max(0, min(len(items)-1, m.cursor+1))
	case "a":
		m.action = MCPAction{Kind: "add"}
		m.done = true
		return m, tea.Quit
	case "enter", "e", "d":
		if len(items) == 0 {
			return m, nil
		}

		switch key.String() {
		case "enter":
			m.detail = true
			return m, m.beginDetails()
		case "d":
			m.confirm = true
		case "e":
			m.action = MCPAction{Kind: "edit", Name: items[m.cursor].Name}
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// detailLines 将详情按终端列宽折行，长地址可以通过滚动完整查看。
func (m mcpModel) detailLines() []string {
	items := m.matches()
	if len(items) == 0 {
		return nil
	}

	text := m.remoteText(items[m.cursor])
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ansi.Hardwrap(clip(line, len(line)*2+1), max(1, m.width-2), true)
	}

	return strings.Split(strings.Join(lines, "\n"), "\n")
}

// View 列表随选中项分页，详情独立滚动；底部始终保留 Esc 提示。
func (m mcpModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	heading := "◆ MCP · 已配置 · 未连接"
	if m.detail {
		heading = "◆ MCP · 服务详情"
	}
	lines := []string{accent.Render(clip(heading, m.width-2))}
	footer := "↑↓ 选择 · Enter 详情 · a 添加 · e 编辑 · d 删除 · / 搜索 · Esc 返回"
	items := m.matches()

	switch {
	case m.confirm:
		lines = append(lines, "", clip("删除服务 "+items[m.cursor].Name+"？", m.width-2))
		footer = "Esc 取消 · Enter 删除"
	case m.detail:
		body := m.detailLines()
		start := min(m.offset, max(0, len(body)-max(1, m.height-2)))
		end := min(len(body), start+max(1, m.height-2))
		lines = append(lines, body[start:end]...)
		footer = fmt.Sprintf("Esc 返回 · r 重试 · ↑↓ · %d/%d", end, len(body))
	default:
		lines = append(lines, ansi.Truncate(m.filter.View(), m.width-2, "…"))
		pageSize := max(1, (m.height-5)/2)
		start := m.cursor / pageSize * pageSize
		if len(items) == 0 {
			lines = append(lines, clip("暂无服务，按 a 添加", m.width-2))
		}

		for i := start; i < min(len(items), start+pageSize); i++ {
			prefix := "  "
			if i == m.cursor {
				prefix = "› "
			}

			name := clip(prefix+items[i].Name, m.width-2)
			if i == m.cursor {
				name = accent.Render(name)
			}

			lines = append(lines, name)
			if m.height > 5 {
				lines = append(lines, muted.Render(clip("  "+items[i].Description, m.width-2)))
			}
		}

		if m.notice != "" {
			lines = append(lines, bad.Render(clip(m.notice, m.width-2)))
		}

		if m.searching {
			footer = "Esc 返回 · Enter 确认搜索"
		}
	}

	if m.width < 70 && !m.confirm && !m.detail && !m.searching {
		footer = "Esc 返回 · a/e/d 增改删 · / 搜索"
	}

	lines = append(lines, muted.Render(clip(footer, m.width-2)))
	return tea.NewView(panel.Width(m.width).Render(strings.Join(lines, "\n")))
}

// ShowMCPDetail 展示可滚动的完整服务详情，Esc 直接返回聊天。
func ShowMCPDetail(server mcp.Server, path string) error {
	model := newMCP([]mcp.Server{server}, path)
	model.detail = true
	model.detailOnly = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model.ctx = ctx
	model.beginDetails()
	_, err := tea.NewProgram(model).Run()
	return err
}
