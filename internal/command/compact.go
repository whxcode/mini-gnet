package command

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/internal/compact"
	"github.com/whxcode/mini-gnet/internal/session"
	menus "github.com/whxcode/mini-gnet/views/commands"
	"github.com/whxcode/mini-gnet/views/message"
)

// handleCompact 总结、保存新会话、切换按顺序完成；界面只等待结果，不承担模型执行。
func handleCompact(parent context.Context, a Application, _ []string) error {
	source, history, err := compactSource(a)
	if err != nil {
		return err
	}

	cfg := a.ConfigStore().Current()
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()

	// 只有总结请求放到后台，让 Esc 能取消。done 关闭后才读取结果并创建文件。
	var summary string
	var summaryErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		summary, summaryErr = compact.Generate(ctx, cfg, history)
	}()

	canceled, viewErr := menus.WaitCompact(ctx, done)
	if canceled || viewErr != nil {
		cancel()
	}
	<-done

	if viewErr != nil {
		return viewErr
	}
	if canceled {
		return nil
	}
	if summaryErr != nil {
		return summaryErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	next, err := session.NewSummaryHistory(cfg, summary, source)
	if err != nil {
		return err
	}
	a.AttachSession(next)
	return nil
}

// compactPipe 同步总结并打印结果；复用相同的校验和保存逻辑，不启动终端菜单。
func compactPipe(parent context.Context, a Application, output io.Writer) error {
	source, history, err := compactSource(a)
	if err != nil {
		return err
	}

	cfg := a.ConfigStore().Current()
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()

	if _, err := fmt.Fprintln(output, "正在总结当前会话…"); err != nil {
		return err
	}
	summary, err := compact.Generate(ctx, cfg, history)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	next, err := session.NewSummaryHistory(cfg, summary, source)
	if err != nil {
		return err
	}
	a.AttachSession(next)

	_, err = fmt.Fprintf(output,
		"已创建摘要续聊会话：%s\n来源会话：%s\n\n%s\n",
		next.SessionID, source.SessionID, summary,
	)
	return err
}

// compactSource 读取空闲会话的历史；命令执行期间主输入循环不会再向它入队。
// 待执行任务也必须排空，避免摘要遗漏马上写入的消息。
func compactSource(a Application) (message.SummarySource, []openai.ChatCompletionMessage, error) {
	s := a.ActiveSession()
	state := s.Snapshot()
	if state.Running || len(state.Pending) > 0 {
		return message.SummarySource{}, nil, fmt.Errorf("当前会话仍有执行或等待任务，请等待完成，或 /stop 后再 /compact")
	}

	history := s.History.ToChatCompletionMessage()
	hasConversation := false
	for _, msg := range history {
		if msg.Role != openai.ChatMessageRoleSystem {
			hasConversation = true
			break
		}
	}
	if !hasConversation {
		return message.SummarySource{}, nil, fmt.Errorf("当前会话还没有可总结的对话")
	}

	source := message.SummarySource{
		SessionID: state.ID,
		Title:     state.Title,
	}
	return source, history, nil
}
