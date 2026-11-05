package commands

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/mcp"
)

// mcpKey 推进一次菜单按键，测试不启动真实终端或网络。
func mcpKey(m mcpModel, code rune) mcpModel {
	next, _ := m.Update(tea.KeyPressMsg{Code: code})
	return next.(mcpModel)
}

// mcpItems 提供带中文和长地址的列表，覆盖窄窗口与折行。
func mcpItems() []mcp.Server {
	var items []mcp.Server
	for i := range 20 {
		items = append(items, mcp.Server{
			Name:        fmt.Sprintf("docs-%d", i),
			Description: "中文简介：测试文档服务的内容与用途",
			Transport:   mcp.StreamableHTTP,
			URL:         "https://example.invalid/" + strings.Repeat("long-path/", 20) + "mcp",
		})
	}
	return items
}

// TestMCPMenuActions 验证选择、搜索、编辑、添加和删除确认，Esc 不返回修改操作。
func TestMCPMenuActions(t *testing.T) {
	m := newMCP(mcpItems(), "/tmp/mcp.json")
	m = mcpKey(m, tea.KeyDown)
	m = mcpKey(m, tea.KeyEnter)
	if !m.detail || m.cursor != 1 {
		t.Fatal("Enter 未显示选中服务详情")
	}
	m = mcpKey(m, tea.KeyEscape)
	if m.done || m.detail {
		t.Fatal("详情 Esc 应返回列表")
	}

	m = mcpKey(m, 'd')
	if !m.confirm || m.done {
		t.Fatal("删除没有经过确认")
	}
	m = mcpKey(m, tea.KeyEscape)
	if m.action.Kind != "" || m.confirm {
		t.Fatal("取消删除仍返回了操作")
	}
	confirmed := mcpKey(mcpKey(m, 'd'), tea.KeyEnter)
	if !confirmed.done || confirmed.action.Kind != "remove" || confirmed.action.Name != "docs-1" {
		t.Fatal("确认删除结果错误")
	}
	if got := mcpKey(m, 'e'); got.action.Kind != "edit" || got.action.Name != "docs-1" {
		t.Fatal("编辑选择错误")
	}
	if got := mcpKey(m, 'a'); got.action.Kind != "add" {
		t.Fatal("添加操作错误")
	}

	m = mcpKey(m, '/')
	if !m.searching || !m.filter.Focused() {
		t.Fatal("搜索输入框未获得焦点")
	}
	next, _ := m.Update(tea.PasteMsg{Content: "docs-19"})
	m = next.(mcpModel)
	if len(m.matches()) != 1 || m.cursor != 0 {
		t.Fatal("搜索未过滤或未重置选中项")
	}
	m = mcpKey(m, tea.KeyEnter)
	if m.searching {
		t.Fatal("搜索确认后未退出输入状态")
	}
	if got := mcpKey(m, tea.KeyEscape); !got.done || got.action.Kind != "" {
		t.Fatal("Esc 未返回聊天")
	}
	if got := mcpKey(newMCP(nil, ""), tea.KeyEnter); got.done || got.detail {
		t.Fatal("空列表不应选中")
	}
}

// assertMCPFits 检查每行显示宽度、总高度以及始终可见的返回提示。
func assertMCPFits(t *testing.T, view tea.View, width, height int) {
	t.Helper()
	lines := strings.Split(view.Content, "\n")
	if len(lines) > height {
		t.Fatalf("菜单高度 %d 超过 %d：\n%s", len(lines), height, view.Content)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > width {
			t.Fatalf("行宽 %d 超过 %d：%s", ansi.StringWidth(line), width, line)
		}
	}
	if !strings.Contains(ansi.Strip(view.Content), "Esc") {
		t.Fatal("返回提示被隐藏")
	}
}

// TestMCPSmallWindowAndScrolling 验证中文布局、长地址滚动、窗口缩放和返回。
func TestMCPSmallWindowAndScrolling(t *testing.T) {
	for _, width := range []int{24, 40, 80} {
		for _, height := range []int{5, 8, 24} {
			m := newMCP(mcpItems(), "/配置/一个很长的路径/mcp.json")
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
			m = next.(mcpModel)
			m.cursor = 19
			m.notice = "测试失败信息"
			assertMCPFits(t, m.View(), width, height)

			m = mcpKey(m, tea.KeyEnter)
			assertMCPFits(t, m.View(), width, height)
			for range 100 {
				m = mcpKey(m, tea.KeyPgDown)
			}
			assertMCPFits(t, m.View(), width, height)
			body := m.detailLines()
			if m.offset != max(0, len(body)-(height-2)) || !strings.Contains(ansi.Strip(m.View().Content), body[len(body)-1]) {
				t.Fatal("长地址末尾无法查看")
			}
			for range 100 {
				m = mcpKey(m, tea.KeyPgUp)
			}
			if m.offset != 0 {
				t.Fatal("未回到顶部")
			}
		}
	}
}

// TestMCPFormValidationAndEscape 验证表单保留错误草稿，确认后返回值，Esc 不保存。
func TestMCPFormValidationAndEscape(t *testing.T) {
	original := mcpItems()[0]
	m := newMCPForm(original)
	m.fields[0].SetValue("changed")
	m.focus = len(m.fields)
	m.fields[2].SetValue("invalid")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(mcpForm)
	if m.done || m.saved != nil || m.problem == "" || m.fields[0].Value() != "changed" {
		t.Fatal("校验失败丢失草稿")
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := next.(mcpForm); !got.done || got.saved != nil {
		t.Fatal("Esc 不应返回待保存配置")
	}
	m.fields[2].SetValue("https://example.invalid/new")
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := next.(mcpForm); !got.done || got.saved == nil || got.saved.Name != "changed" {
		t.Fatal("有效表单未保存")
	}
	if original.Name != "docs-0" {
		t.Fatal("修改了源配置")
	}
}

// TestMCPFormSmallWindow 验证短窗口逐字段显示和光标编辑。
func TestMCPFormSmallWindow(t *testing.T) {
	m := newMCPForm(mcpItems()[0])
	m.problem = "上次保存失败，请修正后重试"
	for _, height := range []int{5, 8, 24} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: 24, Height: height})
		m = next.(mcpForm)
		for range 4 {
			assertMCPFits(t, m.View(), 24, height)
			next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			m = next.(mcpForm)
		}
	}
	if !m.fields[0].Focused() {
		t.Fatal("字段切换没有恢复编辑焦点")
	}
	before := m.fields[0].Value()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = next.(mcpForm)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(mcpForm)
	if m.fields[0].Value() != before[:len(before)-1] {
		t.Fatal("光标移动或删除未生效")
	}
}

// TestConfigPreservesMCPPath 验证通用配置表单不会在保存模型时丢弃 MCP 路径。
func TestConfigPreservesMCPPath(t *testing.T) {
	cfg := config.Config{
		BaseURL:       "http://localhost:11434/v1",
		Model:         "test",
		ChatsDir:      t.TempDir(),
		MCPConfigPath: t.TempDir() + "/mcp.json",
	}
	m := newConfig(cfg)
	m.focus = len(m.fields)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := next.(configModel)
	if got.saved == nil || got.saved.MCPConfigPath != cfg.MCPConfigPath {
		t.Fatal("通用配置表单丢失 MCP 路径")
	}
}

// TestConfigMCPFieldSmallWindow 验证新增长路径字段在窄窗口可编辑，光标和返回提示仍可见。
func TestConfigMCPFieldSmallWindow(t *testing.T) {
	m := newConfig(config.Config{MCPConfigPath: "/很长的配置路径/" + strings.Repeat("目录/", 30) + "mcp.json"})
	m.fields[0].Blur()
	m.focus = 7
	m.fields[7].Focus()
	m.err = "测试保存错误"

	for _, size := range []tea.WindowSizeMsg{
		{Width: 24, Height: 5},
		{Width: 40, Height: 12},
		{Width: 80, Height: 24},
	} {
		next, _ := m.Update(size)
		m = next.(configModel)
		view := m.View()
		assertMCPFits(t, view, size.Width, size.Height)
		if view.Cursor == nil || view.Cursor.Position.Y >= size.Height || view.Cursor.Position.X >= size.Width {
			t.Fatal("配置光标超出可见区域")
		}
	}
}

// TestMCPDetailsLoadingAndCancel 验证 Enter 生成异步请求、显示结果并在 Esc 时取消，旧结果不会覆盖新页面。
func TestMCPDetailsLoadingAndCancel(t *testing.T) {
	m := newMCP(mcpItems(), "/tmp/mcp.json")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(mcpModel)
	if cmd == nil || !m.loading || m.requestContext == nil {
		t.Fatal("详情没有异步加载")
	}
	ctx := m.requestContext
	generation := m.generation
	next, _ = m.Update(mcpDetailsMsg{generation: generation, details: &mcp.Details{ServerName: "server", Protocol: "2025-06-18"}})
	m = next.(mcpModel)
	if m.loading || m.remote == nil || !strings.Contains(m.remoteText(m.servers[0]), "2025-06-18") {
		t.Fatal("真实远程信息未显示")
	}
	m = mcpKey(m, tea.KeyEscape)
	if ctx.Err() == nil {
		t.Fatal("Esc 没有取消请求")
	}
	next, _ = m.Update(mcpDetailsMsg{generation: generation, err: fmt.Errorf("旧请求")})
	m = next.(mcpModel)
	if m.remoteError != "" {
		t.Fatal("退出后旧响应改变了状态")
	}
	m = mcpKey(m, tea.KeyEnter)
	defer m.cancel()
	next, _ = m.Update(mcpDetailsMsg{generation: m.generation, err: fmt.Errorf("服务不可达")})
	m = next.(mcpModel)
	if !strings.Contains(m.remoteText(m.servers[0]), "服务不可达") {
		t.Fatal("错误未展示")
	}
	m = mcpKey(m, 'r')
	defer m.cancel()
	if !m.loading || m.remoteError != "" {
		t.Fatal("重试没有重置加载状态")
	}
}
