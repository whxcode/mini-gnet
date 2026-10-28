package conversation

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestPreviewTwoDisplayLines 验证中文折行与控制字符处理后仍只显示最后两行。
func TestPreviewTwoDisplayLines(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
		want  string
	}{
		{"", 10, "\n"},
		{"第一行\n第二行\n第三行", 20, "第二行\n第三行"},
		{"你好世界再见", 4, "世界\n再见"},
		{"a\x1b[2Jb\r\nc", 10, "ab\nc"},
	} {
		if got := preview(tc.text, tc.width); got != tc.want {
			t.Errorf("preview(%q)=%q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestMessageRoleAndContent 验证角色标题和多行正文完整显示。
func TestMessageRoleAndContent(t *testing.T) {
	for _, role := range []Role{User, Assistant, Tool, System, Error} {
		got := ansi.Strip(Format(Event{Role: role, Name: "名字", Text: "第一行\n第二行"}, 80))
		if !strings.Contains(got, string(role)+" · 名字\n") || !strings.Contains(got, "│ 第一行\n│ 第二行") {
			t.Fatalf("角色或多行正文缺失: %q", got)
		}
	}
}

// TestStreamCommitAndFinish 验证完整消息提交后清空预览，结束后恢复输入区域。
func TestStreamCommitAndFinish(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := newStream(ctx, cancel, nil)
	next, _ := m.Update(Event{Kind: Delta, Text: "你好"})
	m = next.(streamModel)
	if !strings.Contains(ansi.Strip(m.View().Content), "你好") {
		t.Fatal("没有显示生成中的文本")
	}
	next, cmd := m.Update(Event{Role: Assistant, Text: "你好"})
	m = next.(streamModel)
	if m.live != "" || cmd == nil {
		t.Fatal("完整回答没有转入记录，或底部仍重复显示它")
	}
	next, _ = m.Update(finished{})
	if next.(streamModel).View().Content != "" {
		t.Fatal("完成后未清理底部预览")
	}
}

// TestStreamTaskAndCancellation 验证独立生产者与界面配合，正常完成和取消都能退出。
func TestStreamTaskAndCancellation(t *testing.T) {
	for _, stop := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		started := make(chan struct{})
		events := make(chan Event, 8)
		// 应用先启动生产者，界面仅接收消息；取消后生产者关闭通道。
		go func() {
			defer close(events)
			close(started)
			events <- Event{Kind: Delta, Text: "部分回答"}
			if stop {
				<-ctx.Done()
				return
			}
			events <- Event{Role: Assistant, Text: "部分回答"}
		}()
		p := tea.NewProgram(newStream(ctx, cancel, events), tea.WithInput(nil),
			tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
		if stop {
			go func() {
				<-started
				p.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			}()
		}
		result, err := p.Run()
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		got := result.(streamModel)
		if !got.done || (stop && !errors.Is(got.err, context.Canceled)) || (!stop && got.err != nil) {
			t.Fatalf("任务结束或取消处理错误: done=%v, err=%v", got.done, got.err)
		}
	}
}
