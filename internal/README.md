# 模块与阅读顺序

项目保留一个 `go.mod`，按职责划分独立的 Go package。拆包依靠单向依赖，不使用业务回调或循环导入。

```text
internal/
  app/       应用入口、命令分流、会话切换、终端界面
  command/   命令注册、参数解析、handler、管道命令
  session/   会话记录、任务队列、顺序消费、停止与退出
  agent/     模型流、工具调用、有限重试、输出记录
  task/      单次任务的输入、取消信号和完成结果
  skill/     扫描技能目录、解析 SKILL.md 中的名称与简介
```

依赖方向是 `app → session → agent → task`；App 同时负责创建共用的 Agent，Session 也使用 Task。
App 还依赖 Command；Command 通过 Application 接口直接操作传入的 App，不反向导入 App。
Session、Agent、Task 都不导入 App，Agent 不导入 Session 或终端显示包。
配置由 `config.Store` 保存一份，App 和 Agent 共用它；工具注册表也由 App 和 Agent 共用，包含定义与执行入口。

## 按顺序读代码

1. `app/app.go`：加载配置和工具、创建 Agent；主循环读取输入，命令直接调用 handler，普通提问调用 `session.Enqueue(ctx, query)`。
2. `app/workspace.go`：首次创建或恢复会话时调用 `session.New`，然后明确执行一次 `go s.Run()`。切换已有会话只修改当前会话指针。
3. `session/session.go`：`Enqueue` 入队；`Run` 取出一个任务，依次标记开始、调用 Agent、记录结束，再取下一项。
4. `agent/agent.go`：`Agent.Run` 保存提问，发起模型请求，逐段读取流，保存回答，执行工具；需要继续时进入下一轮。
5. `task/task.go`：任务取消和完成通知。Task 不持有配置、客户端或工具列表。

本地命令分支先读 `command/registry.go`，再读 `command/handlers.go`；菜单渲染仍在 `views/commands`。

会话循环的核心顺序是：

```go
for job := range s.taskQueue {
    // 检查取消并标记开始。
    err := s.runner.Run(job, s.History, s.Output)
    s.finishTask(job, err)
}
```

`Agent.Run` 普通返回错误，Session 再调用 `finishTask`；没有把收尾函数传入 Agent，也没有输出回调链。
Agent 可以脱离会话队列单独调用，`agent/agent_test.go` 演示了这种用法。

## 各包管理什么

| 位置 | 职责 |
| --- | --- |
| `app/app.go`、`pipe.go` | 交互输入与管道入口；管道同样入队，但等待本项 `Done` 后再读下一行 |
| `command/registry.go`、`handlers.go` | 命令注册和直接操作应用的 handler |
| `command/application.go`、`app/command_access.go` | 命令需要的应用操作接口及 App 的实现 |
| `command/pipe.go` | 管道命令的文本行为，不启动菜单 |
| `app/terminal.go`、`session_grid.go` | 显示与编辑，不执行模型任务 |
| `session/session.go` | 创建、入队、消费、结束、停止、关闭 |
| `session/display.go` | 读取会话状态快照、清除未读标记 |
| `session/history.go` | 创建历史文件、恢复显示内容 |
| `agent/agent.go` | 模型与工具的顺序执行流程及统一失败处理 |
| `tools/registry.go`、`tools/builtin/register.go`（项目根目录） | 统一工具接口、schema 校验、内置工具注册 |
| `agent/event.go`、`output.go` | 输出事件及带锁的输出记录；视图读取快照后排版 |
| `task/task.go` | 本项输入、取消、完成通知及最终错误 |
| `config/store.go` | 唯一运行配置的并发读写 |

`Session.History` 是发给模型的历史并写入 JSONL；`Session.Output` 是显示记录，会包含错误、进度和未完成文字。
Agent 直接写入 Output，界面读取同一个对象；没有转发消息的 channel 或额外收集协程。
界面的颜色、图标和用户名补全由 `views/conversation` 处理。

## 队列与生命周期

每个会话只有一个消费协程，同一会话顺序执行，不同会话可以并行。
最多等待 16 项；满时立即提示并保留草稿，不阻塞输入界面。
提问到执行时才写入历史，当前任务不会提前读到后续排队问题。
界面单独显示等待数量和下一项提问。队列仅在内存中，退出后不恢复未执行项。

任务出错会记录错误并结束本项，随后继续消费下一项。
`/stop` 取消当前会话的执行任务和全部等待项，其他会话继续执行；之后仍可提交新任务。
`Shutdown` 禁止入队并取消任务；`Close` 等待消费循环退出，再关闭历史文件。
App 先对全部会话调用 Shutdown，再逐一 Close，避免等待一个会话时还没取消其他会话。

`pending` 包含刚被 channel 取出但尚未标记开始的任务，防止 Stop 漏掉这一瞬间的任务。
队列状态和输出记录各自加锁；锁只保护程序内存，没有为共享项目文件添加资源冲突处理。

## 配置与任务字段

修改 `/config` 后，所有尚未开始的任务（包括已排队任务）使用新配置。
Agent 开始一项任务时读取配置到局部变量，本项的工具循环中不会突然更换模型或工作目录。
改变记录目录不会迁移已打开文件；旧会话仍追加到原文件。

Task 的五个字段分别是：`Query` 输入、`Context` 接收取消信号、`cancel` 发出取消信号、`Done` 通知结束、`Err` 保存结果。
消费者取得处理权后调用一次 `Complete`；等待者在 Done 关闭后读取 Err。
`context.Background()` 是不自行取消或超时的根上下文，不会启动线程。
超时时长由 `task.New` 设置，从提交开始计时，排队也计时。

## 工作目录中的项目规则与技能摘要

运行时读取的是工作目录下的 `.agent/AGENTS.md` 和 `.agent/skills/<技能目录>/SKILL.md`，不是项目根目录供开发助手使用的 `AGENTS.md`。工作目录来自 `config.ProjectDir`，未设置时使用启动目录。

`system_propmt.BuildAgentPrompt` 依次组装基础提示词、项目规则正文、技能摘要目录。技能 YAML 中只使用 `name` 和 `description`，并附文件位置；不自动注入正文、执行脚本或注册同名工具。

`Session.NewHistory` 保存创建会话时的系统提示词。`Agent.Run` 在每项任务开始时重新构建提示词，后续工具循环复用这份快照；请求时替换旧的 system 消息，不反复追加。这样恢复旧会话或修改 `.agent` 文件后，下一项任务使用当前规则；同一任务中途不切换规则。

缺少目录或规则文件时正常运行；技能 YAML 损坏、重复名称或文件读取失败时，返回包含路径的错误。最小示例与手动验证步骤见项目根目录的 `.agent/README.md`。

手动入口是 `/skills` 菜单或 `/skills <名称>`：handler 使用 `internal/skill.Load` 读取正文，Session 保存下一条问题的待选技能，成功入队时把正文与问题合成普通 user 消息，并清除待选。

自动入口是注册表中的 `load_skill` 工具：模型根据摘要选择，工具使用同一个加载函数，返回正文与资源目录。Agent 按普通工具调用保存结果，然后继续模型循环；无需在 Agent 中为技能写特殊分支。

两种方式都保留正常历史，不在任务结束时移除技能内容。`/skills --clear` 只取消待选，`/new` 开始不带旧正文的会话。`/resume` 恢复原有 user/tool 消息，无需单独恢复技能状态。

## 多轮响应与最终回答

一次用户提问可以发起多次模型请求。网络层拼接流式内容和工具参数，并保留 `finish_reason`；Agent 读取完整响应后决定下一步：

- `tool_calls` 且确实有工具调用：保存中间轮次，执行工具，把结果交给下一次模型请求。
- `stop` 且没有工具调用：保存唯一最终回答并结束本项，不根据正文判断用户需求是否解决。
- 如果兼容服务返回 `stop` 同时携带工具调用，仍先执行工具，不能提前标成最终回答。
- `length`、`content_filter`、缺少结束原因或断流：标记未完成，保留已收到的文字，不执行不完整的工具调用。

工具失败后，下一次模型请求仍携带真实错误供模型修复；模型返回 `stop` 且无工具调用时，不再因为正文“看起来没有完成”而强制追加请求。本项累计三次工具失败和八轮模型调用上限仍保留。

执行中的正文尚不能确定是中间说明还是最终回答，因此先放入过程预览。收到最终 `stop` 后，将这一轮正文移到独立的“assistant · 最终回答”，不重复展示。工具结果与中间轮次仍完整保留。

新 JSONL 在 assistant 消息上增加 `agent_response`，保存结束原因和最终回答标记；请求模型时只发送标准消息字段。旧历史仍可加载，但缺少标记的普通回答不会被伪标为已确认的最终回答。

## 使用

- `/new` 创建新会话；旧任务继续执行。
- `/tools` 查看已注册工具与参数；`/tools run_bash` 查看完整 schema，Esc 返回。
- `/ps 2` 或 `/sessions 2` 切换到编号 2；不带编号或 F2 打开列表，Esc 返回。
- `/resume` 加载历史；已经打开的记录直接切回，不重复创建写入者或消费协程。
- `/stop` 停止当前会话并清空其等待队列。
- `/exit` 在存在执行或等待任务时确认退出，Esc 返回。
- Alt+←/→ 切换、Alt+N 新建；草稿和光标保留。
- Enter 发送，Ctrl+J / Shift+Enter / Alt+Enter 换行。
- 滚轮每次滚动三行；PgUp/PgDown 翻页，Ctrl+Home 到顶部，Ctrl+End 回到最新输出。
- Ctrl+O 查看当前会话的完整执行记录，Esc 返回；详情和聊天各自保留滚动位置，输入草稿不变。

顶部栅格随窗口宽度调整，当前会话高亮，显示状态和未读结果。
输入框随内容增长，短对话不填充到屏幕底部。后台结果在原会话保存，切回只负责展示。
长输出右侧显示滚动位置条，底部显示当前行范围和总行数。向上阅读时新增输出不会推走当前内容，回到底部后恢复跟随。滚动排版在 `views/conversation/transcript.go`，不裁剪会话中的原始消息。
聊天默认将连续的中间轮次、工具调用和结果放进带边框的执行过程，只预览最近六行；最终回答单独显示全文。过程排版位于 `views/conversation/process.go`，详情视图位于 `app/details.go`。管道仍输出完整记录，不折叠。

## 测试

```sh
go test -race ./internal/... ./net ./views/... ./config -timeout 45s
```

`task` 测试独立取消和完成通知；`agent` 测试独立对话和工具执行；`session` 测试队列、取消、Agent 集成和重试；`command` 测试命令解析；`app` 测试命令集成、跨会话切换、退出及 UI。
测试使用本地模拟模型，不请求付费接口。
