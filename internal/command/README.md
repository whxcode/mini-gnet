# 本地命令模块

这里管理 `/help`、`/tools`、`/skills`、`/config`、`/resume`、`/new`、`/sessions`、`/ps`、`/stop`、`/exit`。

- `registry.go`：命令名称、说明、参数限制、handler 注册，以及 `Lookup` 和帮助文本。
- `handlers.go`：执行各命令，完成操作后只返回错误。
- `tools.go`：从实际注册表生成 /tools 列表和定义详情。
- `skills.go`：选择并加载下一条提问的技能；不发起模型请求或直接写入历史。
- `application.go`：命令需要的应用操作；App 实现此接口。
- `pipe.go`：管道中的文本帮助、会话列表和菜单不可用提示。

交互主循环直接执行：

```go
cmd, args, err := command.Lookup(query)
if err != nil {
    // 显示错误。
    continue
}
if cmd != nil {
    if err := cmd.Handler(a, args); err != nil {
        // 显示错误。
    }
    continue
}
// 普通提问交给当前 Session 入队。
```

实际传入的 `a` 仍然是 `*app.App`。接口只是列出命令可以调用哪些方法，避免 `app → command → app` 的循环导入；没有把 handler 回调注入 App，也不返回让 App 再解释的操作结果。

命令流程归本包；会话列表和队列的生命周期仍由 App、Session 管理。菜单渲染在 `views/commands`，Esc 返回空选择，handler 随即结束。

增加命令时，在 `registry.go` 添加名称和注册项，在 `handlers.go` 实现 handler。只有需要不同管道行为的命令才需调整 `pipe.go`。
