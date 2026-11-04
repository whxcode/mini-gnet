package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/whxcode/mini-gnet/tools"
)

// ProcessTool 保存一个外部工具的定义和启动配置，所有语言共用这个执行器。
type ProcessTool struct {
	definition tools.Definition
	command    string
	args       []string
	cwd        string
	toolDir    string
	configPath string
}

var _ tools.Tool = (*ProcessTool)(nil)

// Definition 返回公共定义副本，模型和 /tools 都不需要知道执行器类型。
func (t *ProcessTool) Definition() tools.Definition {
	return t.definition.Clone()
}

// DescribeLaunch 返回已加载的启动配置，菜单不会重新读取可能已被修改的文件。
func (t *ProcessTool) DescribeLaunch(_ tools.Context) tools.LaunchInfo {
	return tools.LaunchInfo{
		Command:    t.command,
		Args:       append([]string(nil), t.args...),
		WorkDir:    t.cwd,
		ToolDir:    t.toolDir,
		ConfigPath: t.configPath,
		Note:       "输入参数以 JSON 写入 stdin；stdout 返回 JSON，stderr 用于诊断。脚本相对路径以工作目录为起点。",
	}
}

// Execute 启动进程、写入 JSON 参数、收集输出，再交给注册表校验业务结果。
func (t *ProcessTool) Execute(ctx context.Context, request tools.Request) (tools.Result, error) {
	processContext, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(processContext, t.command, t.args...)
	cmd.Dir = t.cwd
	cmd.Stdin = bytes.NewReader(request.Arguments)
	cmd.WaitDelay = time.Second

	// 超过输出上限时停止子进程，避免持续输出占满内存。
	stdout := limitedOutput{
		limit:  1 << 20,
		cancel: cancel,
	}
	stderr := limitedOutput{
		limit:  64 << 10,
		cancel: cancel,
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	execution, runErr := tools.RunProcess(cmd)
	result := tools.Result{Execution: execution}

	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	if stdout.exceeded || stderr.exceeded {
		return result, &tools.ToolError{
			Code:    tools.InvalidOutput,
			Message: "工具输出超过限制：stdout 最多 1 MiB，stderr 最多 64 KiB",
		}
	}

	if runErr != nil {
		return result, fmt.Errorf(
			"启动或执行工具 %s 失败：%w；stderr：%s",
			t.definition.Name,
			runErr,
			strings.TrimSpace(stderr.buffer.String()),
		)
	}

	// stdout 只承载一个 JSON 值；空输出、日志混入和错误结构由注册表统一拒绝。
	result.Data = json.RawMessage(stdout.buffer.Bytes())

	return result, nil
}
