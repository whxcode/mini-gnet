package main

import (
	"context"

	"github.com/whxcode/mini-gnet/internal/app"
	"github.com/whxcode/mini-gnet/logger"
)

// main 只负责启动和关闭应用；输入循环在 app.Run，对话流程在 app.Chat。
func main() {
	application, err := app.New()
	if err != nil {
		logger.Error(err)
		return
	}
	defer application.Close()
	if err := application.Run(context.Background()); err != nil {
		logger.Error(err)
	}
}
