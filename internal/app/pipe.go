package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	commands "github.com/whxcode/mini-gnet/internal/command"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// RunPipe 专用于管道：按行读取、入队并等待本项完成、打印结果；不启动交互菜单。
// 本文件负责读取和输出，命令的管道行为由 command.RunPipe 处理。
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

		command, args, err := commands.Lookup(query)
		if err != nil {
			fmt.Fprintln(output, conversation.Format(conversation.Event{
				Role: conversation.Error,
				Text: err.Error(),
			}, 0))
			continue
		}

		if command != nil {
			// 只有依赖菜单的命令需要文本形式；其余仍直接调用原 handler。
			session := a.CurrentSession
			firstEvent := len(session.Snapshot().Messages)
			err = command.RunPipe(ctx, a, args, output)
			if err != nil {
				fmt.Fprintln(output, conversation.Format(conversation.Event{
					Role: conversation.Error,
					Text: err.Error(),
				}, 0))
			}

			// 打印命令新产生的提示，不把切换目标的已有历史重复输出。
			for _, event := range session.Snapshot().Messages[firstEvent:] {
				fmt.Fprintln(output, conversation.Format(event, 0))
			}

			continue
		}

		session := a.CurrentSession
		firstMessage := len(session.Snapshot().Messages)
		job, err := session.Enqueue(ctx, query)
		if err != nil {
			fmt.Fprintln(output, conversation.Format(conversation.Event{
				Role: conversation.Error,
				Text: err.Error(),
			}, 0))
			continue
		}

		// 等本行任务完成，再读取下一行；EOF 不会提前取消正在执行的请求。
		<-job.Done

		for _, event := range session.Snapshot().Messages[firstMessage:] {
			fmt.Fprintln(output, conversation.Format(event, 0))
		}
	}

	return nil
}
