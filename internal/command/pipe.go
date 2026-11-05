package command

import (
	"fmt"
	"io"

	"github.com/whxcode/mini-gnet/views/contextusage"
)

// RunPipe 执行管道中的命令：菜单使用文本替代，其他操作直接调用同一个 Handler。
// 应用只负责读取输入、调用此方法和输出错误，不维护第二份命令分支。
func (c *Command) RunPipe(a Application, args []string, output io.Writer) error {
	switch {
	case c.Name == ContextName:
		state := a.ActiveSession().Snapshot()
		_, err := fmt.Fprintln(output, contextusage.Detail(state.ID, state.Title, state.Status, state.Usage))

		return err

	case c.Name == MCPName:
		text, err := runMCPCommand(a.MCPManager(), args)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(output, text)
		return err

	case c.Name == SkillsName:
		text, err := skillsText(a.ConfigStore().Current().ProjectDir, args)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(output, text)

		return err

	case c.Name == ToolsName:
		text, err := toolsText(a.ToolInfos(), args)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(output, text)

		return err

	case c.Name == HelpName:
		_, err := fmt.Fprintln(output, HelpText())

		return err
	case c.Name == ConfigName:
		return fmt.Errorf("请在交互终端中使用 /config 编辑配置")
	case c.Name == ResumeName && len(args) == 0:
		return fmt.Errorf("管道中请使用 /resume <会话ID>")
	case (c.Name == SessionsName || c.Name == PSName) && len(args) == 0:
		for i, session := range a.OpenSessions() {
			state := session.Snapshot()

			if _, err := fmt.Fprintf(output, "%d  %s · %s  %s\n", i+1, state.Status, state.Title, state.ID); err != nil {
				return err
			}
		}

		return nil

	case c.Name == ExitName:
		// 管道主循环逐项等待任务完成，不需要交互确认。
		a.RequestExit()

		return nil

	default:
		return c.Handler(a, args)
	}
}
