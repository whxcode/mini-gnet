package session

import (
	"errors"
	"fmt"
	"os"

	"github.com/whxcode/mini-gnet/config"
	prompt "github.com/whxcode/mini-gnet/system_propmt"
	"github.com/whxcode/mini-gnet/views/message"
)

// NewSummaryHistory 先加载当前项目规则，再保存摘要；全部成功后才交给 App 接管。
// 失败时关闭并删除本次新建文件，绝不删除或修改来源会话。
func NewSummaryHistory(
	cfg config.Config,
	summary string,
	source message.SummarySource,
) (history *message.TempMessage, runErr error) {
	systemPrompt, err := prompt.BuildAgentPrompt(cfg.ProjectDir)
	if err != nil {
		return nil, err
	}
	history, err = message.New(cfg.ChatsDir)
	if err != nil {
		return nil, fmt.Errorf("创建摘要会话: %w", err)
	}
	defer func() {
		if runErr != nil {
			runErr = errors.Join(runErr, history.Close(), os.Remove(history.Path()))
			history = nil
		}
	}()

	if err := history.AppendSystemMessage(systemPrompt); err != nil {
		return history, fmt.Errorf("保存摘要会话规则: %w", err)
	}
	if err := history.AppendSummary(summary, source); err != nil {
		return history, fmt.Errorf("保存会话摘要: %w", err)
	}
	return history, nil
}
