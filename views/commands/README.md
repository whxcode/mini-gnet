# 本地命令

在输入框键入命令并按 Enter。这些命令由 Go 程序处理，不发给模型，也不写入聊天正文。

| 命令 | 功能 |
| --- | --- |
| `/help` | 查看命令与快捷键，Esc 或 Enter 返回 |
| `/config` | 编辑接口地址、模型、API Key、聊天记录目录和工作目录 |
| `/resume` | 从当前配置的 `ChatsDir` 读取 `.jsonl` 历史，上下选择、Enter 恢复 |
| `/resume <会话ID>` | 直接恢复指定会话，也可用于管道输入 |
| `/new` | 创建新会话，保留旧记录 |
| `/exit` | 退出程序 |

所有菜单支持 **Esc 返回**；配置修改与会话选择只有确认后才应用。

## 配置

`/config` 中使用 Tab、Shift+Tab 或上下键切换字段，Enter 进入下一项；选中“保存并应用”后按 Enter 才会保存。API Key 隐藏显示。Esc 丢弃本次修改。

配置文件通过 `os.UserConfigDir()` 定位：

- macOS：`~/Library/Application Support/mini-gnet/config.json`
- Linux：`$XDG_CONFIG_HOME/mini-gnet/config.json`，未设置时为 `~/.config/mini-gnet/config.json`
- Windows：`%AppData%\mini-gnet\config.json`

首次没有配置文件时使用 `config/config.go` 的默认值；保存后，文件中的配置优先，下次启动也会加载。配置文件包含 API Key，Unix 下新文件权限为 `0600`，不要提交到 Git。

保存后新的接口、密钥与模型立即生效。改变聊天记录目录或工作目录会开启新会话，避免与原会话混用。工作目录留空表示使用启动程序时的目录。

## 恢复会话

列表按最后修改时间倒序排列，展示第一条用户提问、时间、消息数和会话 ID。还没有用户消息的空会话不展示；损坏记录会标记，不能恢复。

恢复后加载完整历史上下文（含工具调用及结果），继续追加到原来的 `.jsonl` 文件；终端仅回显最近 20 条记录，内部 system 提示词不回显。恢复使用当前配置的模型与工作目录，请选择相应项目的历史记录。

## 阅读代码

1. `commands.go`：识别命令并返回操作意图。
2. `menu.go`、`config.go`：Bubble Tea 的 `Update` 处理按键、`View` 绘制菜单；Esc 返回空结果。
3. `internal/app/session.go` 的 `App.applyLocalCommand`：确认后保存配置或切换会话。
4. `views/message/history.go`：读取、列出和继续写入历史记录。

这种分工让菜单操作可独立测试，按 Esc 不会提前改变当前会话或配置。
