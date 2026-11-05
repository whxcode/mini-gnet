package main

import (
	"context"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/whxcode/mini-gnet/internal/app"
	"github.com/whxcode/mini-gnet/logger"
)

// main 只在启动时判断一次终端环境，选择交互或管道入口；业务流程不再判断运行模式。
func main() {
	application, err := app.MakeApp()
	if err != nil {
		logger.Error(err)

		return
	}

	defer application.Close()

	// Background 只是根上下文，不启动 goroutine，也不会自行超时或取消。
	// 每项任务从它派生自己的取消信号；/stop 和退出时只取消相应任务。
	ctx := context.Background()

	if term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd()) {
		err = application.Run(ctx)
	} else {
		err = application.RunPipe(ctx, os.Stdin, os.Stdout)
	}

	if err != nil {
		logger.Error(err)
	}
}
