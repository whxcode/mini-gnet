# 本地命令

在输入框键入命令并按 Enter。这些命令由 Go 程序处理，不发给模型，也不写入聊天正文。

| 命令 | 功能 |
| --- | --- |
| `/help` | 查看命令与快捷键，Esc 或 Enter 返回 |
| `/tools` | 查看当前已注册工具、说明、参数类型及必填/可选状态 |
| `/tools <工具名>` | 查看完整工具定义，包含输入和输出 schema |
| `/config` | 编辑接口地址、模型、API Key、聊天记录目录和工作目录 |
| `/resume` | 从当前配置的 `ChatsDir` 读取 `.jsonl` 历史，上下选择、Enter 恢复 |
| `/resume <会话ID>` | 直接恢复指定会话，也可用于管道输入 |
| `/new` | 创建并切换新会话，旧任务继续运行 |
| `/sessions` | 切换已打开的会话；可带编号或完整 ID |
| `/ps 2` | 快速切换到顶部卡片编号 2；`/ps` 打开会话列表 |
| `/stop` | 停止当前会话的执行任务并取消全部等待任务 |
| `/exit` | 有执行或等待任务时确认退出，停止并等待所有任务后关闭记录 |

所有菜单支持 **Esc 返回**；配置修改与会话选择只有确认后才应用。

聊天中的执行过程默认预览最近六行，最终回答单独展示。按 **Ctrl+O** 查看当前会话完整执行记录，**Esc** 返回聊天；详情中支持滚轮、PgUp/PgDown、Ctrl+Home/Ctrl+End，后台任务继续执行，草稿保持原样。

本目录只负责交互菜单，不检测终端环境。`main.go` 在启动时选择入口；管道中的文本帮助、会话列表以及菜单不可用提示统一放在 `internal/command/pipe.go`。

## 配置

`/config` 中使用 Tab、Shift+Tab 或上下键切换字段，Enter 进入下一项；选中“保存并应用”后按 Enter 才会保存。API Key 隐藏显示。Esc 丢弃本次修改。

配置文件通过 `os.UserConfigDir()` 定位：

- macOS：`~/Library/Application Support/mini-gnet/config.json`
- Linux：`$XDG_CONFIG_HOME/mini-gnet/config.json`，未设置时为 `~/.config/mini-gnet/config.json`
- Windows：`%AppData%\mini-gnet\config.json`

首次没有配置文件时使用 `config/config.go` 的默认值；保存后，文件中的配置优先，下次启动也会加载。配置文件包含 API Key，Unix 下新文件权限为 `0600`，不要提交到 Git。

配置由 App 统一持有，保存后新的接口、密钥与模型用于所有尚未开始的任务（包括已排队任务）；执行中的任务使用本次开始执行时已读取的参数。改变聊天记录目录或工作目录会开启新会话，旧任务继续运行，已打开的聊天记录仍写入原文件。工作目录留空表示使用启动程序时的目录。

## 恢复会话

列表按最后修改时间倒序排列，展示第一条用户提问、时间、消息数和会话 ID。还没有用户消息的空会话不展示；损坏记录会标记，不能恢复。

恢复后加载完整历史上下文（含工具调用及结果），继续追加到原来的 `.jsonl` 文件；界面可用滚轮或 PgUp/PgDown 查看历史，Ctrl+Home 到顶部，Ctrl+End 回到最新输出。长内容显示滚动条和行数，内部 system 提示词不回显。已打开的记录直接切回；尚未打开的记录使用当前配置加载，不重启历史任务。

## 阅读代码

1. `internal/command/registry.go`：`builtinCommands()` 统一登记命令，`Lookup` 只查找命令与检查参数。
2. `internal/app/app.go`：主循环拿到命令后直接调用 `command.Handler(a, args)`，只处理返回的错误。
3. `internal/command/handlers.go`：每个 handler 接收 `Application` 接口（实际传入 `*App`），完成菜单交互、保存配置或切换会话等全部操作，只返回 `error`。
4. 本目录的 `commands.go`、`menu.go`、`config.go`：只负责菜单显示，向 handler 返回具体选择；Esc 返回空选择。
5. `views/message/history.go`：读取、列出和继续写入历史记录。

handler 位于独立的 `internal/command` 包。`application.go` 列出它需要的应用操作，App 实现这些方法，命令包不导入 App，因此不会循环引用。按 Esc 时 handler 直接返回，不改变当前会话或配置。这里没有通用的 `Result`，也没有再次解释结果的 `applyLocalCommand`。

新增命令时，定义一个命令名称常量、编写对应 handler，再在 `builtinCommands()` 中登记一项即可；无需增加字符串 `switch` 分支。`MaxArgs` 默认为 0，表示不接受参数。注册表使用有序列表，帮助菜单的顺序也由它决定。

会话切换和任务执行的完整说明见 `internal/README.md`。所有命令仍由 handler 直接操作 App，菜单不承担任务执行。
