package bash

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/whxcode/mini-gnet/tools"
)

var allowed = map[string]bool{
	"cat":  true,
	"sed":  true,
	"rg":   true,
	"git":  true,
	"go":   true,
	"pwd":  true,
	"ls":   true,
	"find": true,
	"set":  true,
}

// Run 保留手动调用入口，返回合并后的文本；注册工具使用结构化的 runCommand。
func Run(ctx context.Context, projectDir, command string) (string, error) {
	result, err := runCommand(ctx, projectDir, command)

	return result.Stdout + result.Stderr, err
}

// runCommand 执行一条命令并分别保留 stdout、stderr 和退出码；Dir 不限制绝对路径访问。
func runCommand(ctx context.Context, projectDir, command string) (tools.ProcessOutput, error) {
	result := tools.ProcessOutput{ExitCode: -1}

	if err := validate(command); err != nil {
		return result, err
	}

	// 不加载登录脚本，避免用户 Shell 初始化逻辑改变本次指定的 cwd。
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", command)
	cmd.Dir = projectDir
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	return result, err
}

// validate 保留命令白名单与单行语法限制；具体子命令的操作授权仍由调用方负责。
func validate(command string) error {
	trimmed := strings.TrimSpace(command)

	if trimmed == "" {
		return fmt.Errorf("empty bash command")
	}

	// First version: commands are for inspection and tests only. Chaining is
	// rejected so a permitted command cannot hide a second command.
	if strings.ContainsAny(trimmed, ";|&><`$()\n\r") {
		return fmt.Errorf("command contains unsupported shell syntax")
	}

	fields := strings.Fields(trimmed)

	if len(fields) == 0 {
		return fmt.Errorf("empty bash command")
	}

	if !allowed[fields[0]] {
		return fmt.Errorf("command %q is not allowed", fields[0])
	}

	return nil
}
