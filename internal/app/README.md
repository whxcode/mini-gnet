# 从哪里开始读

这一轮把业务回调改成主动调用，并把入口原本混在一起的职责放进同一个 `app` 包。
先顺着下面的主线阅读，不需要先学习 Bubble Tea 的 `Update` 和 `Cmd`。

```text
main → app.New → App.Run → App.Chat → runAgentTurn
```

| 文件 | 阅读重点 |
| --- | --- |
| 根目录 `main.go` | 启动应用、运行、关闭资源 |
| `app.go` | 初始化、输入循环、命令分流、启动一轮对话 |
| `agent.go` | 请求模型、主动读取回答、执行工具、重试或结束 |
| `tools.go` | 实际执行 Bash/Python，以及将工具数据转换为显示文字 |
| `commands.go` | 命令注册、查找与参数检查 |
| `command_handlers.go` | handler 直接接收 App，完成配置保存、会话切换或退出 |
| `session.go` | 创建会话和回显历史的通用操作 |

## 一轮对话的执行顺序

1. `App.Run` 从输入框读取完整问题。
2. 如果是本地命令，查找后直接调用 `command.Handler(a, args)`；handler 完成操作，只返回错误，然后回到输入。
3. 普通问题进入 `App.Chat`，先保存用户消息。
4. `runAgentTurn` 取出当前会话消息，调用 `readAnswer`。
5. `readAnswer` 调用模型客户端的 `OpenStream`，拿到响应流。
6. 在 `for` 循环中调用 `Next()`，主动读取每段文字；有新文字就发送显示消息。
7. 遇到 `io.EOF` 表示本次回答读完，取得完整文字和工具调用，并关闭响应流。
8. 保存模型回答；没有工具调用时结束，有工具调用时依次执行、保存结果，再请求模型。
9. Python 执行失败时保留原有的有限重试行为；重试耗尽时返回错误。
10. Agent 返回后，应用关闭显示消息通道，界面退出预览，再显示输入框。

这条业务路径使用普通函数调用和返回值，没有“把业务函数传给界面，等界面回调执行”。
模型客户端也不接收文字回调，而是提供 `OpenStream / Next / Result / Close`。

## 为什么 App.Chat 里还有一个 goroutine

模型读取与工具执行可能等待数秒，期间仍要刷新文字、监听 Ctrl+C。
因此 `App.Chat` 将 Agent 放在一个工作协程中，当前协程运行界面。

```text
App.Chat
  启动 Agent 工作协程 ──显示消息──→ 界面读取并刷新
  Agent 顺序执行业务              界面监听 Ctrl+C
  Agent 完成并关闭消息通道        界面收到通道关闭后退出
  等待 Agent 结束 → 返回输入
```

这里只有显示需要并发，模型和工具的业务步骤仍按顺序执行。

- `events`：Agent 发送显示消息，界面只读取。
- `done`：Agent 返回本轮错误；应用等待它，确保上一轮不会继续写记录。
- `context`：Ctrl+C 或界面异常退出时，应用取消网络请求和工具执行。

Bubble Tea 内部仍要求 `Cmd` 函数，相关适配留在 `views/conversation/stream.go`；它只读取消息和刷新动画，不再启动 Agent。

## 会话为什么放在 App 中

配置、客户端和当前会话由同一个 `App` 持有。切换时可以直接看到：

```go
a.session.Close()
a.session = next
```

不再传递 `**TempMessage` 让另一层间接修改调用者的变量。新会话读取成功后才替换旧会话；Esc 时 handler 直接返回，因此不会切换。

命令 handler 的签名是 `func(app *App, args []string) error`。例如 `/resume` 的 handler 自己选择历史、读取记录、关闭旧会话并更新 `app.session`，主循环不再接收 `Result` 后二次执行。`/exit` 的 handler 设置 `app.quit`，主循环退出并由 `main` 关闭资源。

## 本轮验证

```sh
go test -race ./internal/app ./net ./views/... ./config
```

原来根目录的业务测试已搬到 `internal/app`。测试覆盖文字逐段显示、工具执行后继续请求、Python 修复与重试上限、会话恢复以及取消任务。网络测试使用本地模拟 HTTP 服务，不请求真实模型。

本轮保留现有菜单、输入框和会话文件格式。后续可以单独整理会话存储的目录名称与 Agent 重试策略，避免同时更改太多行为。
