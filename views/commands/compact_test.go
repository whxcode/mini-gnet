package commands

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestCompactWaitEscapeAndLayout 验证窄窗口折行、Esc 取消意图和完成后返回。
func TestCompactWaitEscapeAndLayout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	m := compactModel{ctx: ctx, done: done, width: 78}
	for _, width := range []int{24, 40, 80} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 12})
		m = next.(compactModel)
		plain := ansi.Strip(m.View().Content)
		if !strings.Contains(plain, "Esc") || len(strings.Split(plain, "\n")) > 12 {
			t.Fatal("等待提示无法显示", plain)
		}
		for _, line := range strings.Split(plain, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("等待界面超宽", line)
			}
		}
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !next.(compactModel).canceled || cmd == nil {
		t.Fatal("Esc 未请求取消")
	}
	close(done)
	msg := m.Init()()
	next, cmd = m.Update(msg)
	if next.(compactModel).canceled || cmd == nil {
		t.Fatal("正常完成被当成取消")
	}
}

// TestCompactWaitCancellationReleasesReader 验证退出菜单时 Init 不再等待未结束的任务。
func TestCompactWaitCancellationReleasesReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := compactModel{ctx: ctx, done: make(chan struct{})}
	finished := make(chan struct{})
	go func() {
		m.Init()()
		close(finished)
	}()
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("取消后菜单等待未退出")
	}
}
