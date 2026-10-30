package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/whxcode/mini-gnet/views/conversation"
)

// RunPipe 专用于管道：按行读取、顺序执行、打印结果；不启动菜单或后台任务。
// 管道的特殊处理集中在本文件，读交互主流程时可以完全跳过这里。
func (a *App) RunPipe(ctx context.Context, input io.Reader, output io.Writer) error {
	reader := bufio.NewReader(input)
	for !a.quit {
		if err := ctx.Err(); err != nil {
			return err
		}
		query, err := reader.ReadString('\n')
		if errors.Is(err, io.EOF) && len(query) == 0 {
			return nil
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		// 最后一行即使没有换行符，也先执行；下一次读取空 EOF 才结束。
		if strings.TrimSpace(query) == "" {
			continue
		}
		command, args, err := lookupCommand(query)
		if err != nil {
			fmt.Fprintln(output, conversation.Format(conversation.Event{Role: conversation.Error, Text: err.Error()}, 0))
			continue
		}
		if command != nil {
			// 只有依赖菜单的命令需要文本形式；其余仍直接调用原 handler。
			session := a.CurrentSession
			firstEvent := len(session.snapshot().Messages)
			switch {
			case command.Name == commandHelpName:
				fmt.Fprintln(output, commandHelp())
			case command.Name == commandConfigName:
				err = fmt.Errorf("请在交互终端中使用 /config 编辑配置")
			case command.Name == commandResumeName && len(args) == 0:
				err = fmt.Errorf("管道中请使用 /resume <会话ID>")
			case (command.Name == commandSessionsName || command.Name == commandPSName) && len(args) == 0:
				for i, session := range a.opened {
					state := session.snapshot()
					fmt.Fprintf(output, "%d  %s · %s  %s\n", i+1, state.Status, state.Title, state.ID)
				}
			case command.Name == commandExitName:
				a.quit = true // 管道任务已同步执行完毕，无需交互确认。
			default:
				err = command.Handler(a, args)
			}
			if err != nil {
				fmt.Fprintln(output, conversation.Format(conversation.Event{Role: conversation.Error, Text: err.Error()}, 0))
			}
			// 打印命令新产生的提示，不把切换目标的已有历史重复输出。
			for _, event := range session.snapshot().Messages[firstEvent:] {
				fmt.Fprintln(output, conversation.Format(event, 0))
			}
			continue
		}

		session := a.CurrentSession
		firstMessage := len(session.snapshot().Messages)
		job, err := a.prepareTask(ctx, query)
		if err != nil {
			fmt.Fprintln(output, conversation.Format(conversation.Event{Role: conversation.Error, Text: err.Error()}, 0))
			continue
		}
		// 等本行任务完成，再读取下一行；EOF 不会提前取消正在执行的请求。
		session.RunTask(job)
		for _, event := range session.snapshot().Messages[firstMessage:] {
			fmt.Fprintln(output, conversation.Format(event, 0))
		}
	}
	return nil
}
