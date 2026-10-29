package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestPipeCommandsStayTextOnly 验证管道里的菜单命令采用文本形式、普通命令仍能切换会话，且不调用模型。
func TestPipeCommandsStayTextOnly(t *testing.T) {
	a := terminalTestApp(t)
	first := a.CurrentSession
	var output bytes.Buffer
	input := "/help\n/config\n/resume\n/new\n/sessions\n/sessions 1\n/stop\n/unknown\n/exit\n不应执行"
	if err := a.RunPipe(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/help", "交互终端", "/resume <会话ID>", "1  空闲", "2  空闲", "没有正在执行的任务", "未知命令"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("缺少管道反馈 %q：%s", want, output.String())
		}
	}
	if a.CurrentSession != first || len(a.opened) != 2 || !a.quit {
		t.Fatal("管道命令没有正确新建、切换或退出")
	}
	for _, s := range a.opened {
		if len(s.history.ToChatCompletionMessage()) != 1 {
			t.Fatal("命令或退出后的内容被写入模型上下文")
		}
	}
}

// TestPipeFinalCommandWithoutNewline 验证复用读取缓冲，不遗漏最后无换行的命令，也不会把 EOF 当作命令错误打印。
func TestPipeFinalCommandWithoutNewline(t *testing.T) {
	a := terminalTestApp(t)
	var output bytes.Buffer
	if err := a.RunPipe(context.Background(), strings.NewReader("/new\n/sessions"), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "2  空闲") || strings.Contains(output.String(), "EOF") {
		t.Fatalf("最后一行处理错误：%s", output.String())
	}
}
