# 应用层

本包只负责应用组装、输入入口、命令分流、会话切换和终端界面。

- `app.go`：初始化共享配置和 Agent、读取输入、退出。
- `command_access.go`：向命令包提供共享配置、当前会话和退出操作。
- `../command/`：命令注册、解析和 handler，App 只查找并执行命令。
- `workspace.go`：新建、恢复、切换会话，首次打开时启动消费循环。
- `pipe.go`：管道输入，入队后等待任务完成。
- `terminal.go` / `session_grid.go`：输入编辑和栅格显示。
- `details.go`：Ctrl+O 完整执行记录与独立滚动，Esc 返回聊天。

会话、模型循环和任务分别位于 `../session`、`../agent`、`../task`，本包不实现这些职责。
完整依赖图、逐步阅读顺序、队列规则及测试命令见 [模块说明](../README.md)。
