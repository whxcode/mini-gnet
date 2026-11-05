# MCP 配置、发现与调用

本包负责服务配置的加载、校验、增删改查和保存。主配置 `config.json` 只增加 `mcpConfigPath` 路径字段，服务内容单独保存在 `mcp.json`，不使用数据库。

支持 `streamable_http` 服务的初始化、工具列表发现和工具调用，协议由官方 Go SDK 处理，支持 HTTP JSON 和 SSE 响应。暂不支持 stdio、OAuth/自定义认证配置或旧版独立 SSE 端点。

列表中的“已配置 · 未连接”表示没有驻留连接。按 Enter 打开详情后，会异步获取服务名称、版本、协议版本、说明和全部工具的输入/输出 schema。失败显示真实错误，`r` 重试，Esc 返回并取消等待。获取完成就关闭详情连接，页面保留这一次结果快照。

## 文件位置与格式

未设置 `mcpConfigPath` 时，默认与平台的 `config.json` 放在同一目录：

- macOS：`~/Library/Application Support/mini-gnet/mcp.json`
- Linux：`$XDG_CONFIG_HOME/mini-gnet/mcp.json`，未设置时使用 `~/.config/mini-gnet/mcp.json`
- Windows：`%AppData%\mini-gnet\mcp.json`

可以在 `/config` 修改路径，重启后加载。自定义路径支持 `~`；相对路径以程序启动目录为起点。启动时文件不存在会显示空列表，首次成功添加才创建文件。手动修改文件后需要重启；损坏、包含未知字段或不支持的配置会明确报错，不会覆盖原文件。

```json
{
  "version": 1,
  "servers": [
    {
      "name": "microsoft-learn",
      "description": "微软官方文档",
      "transport": "streamable_http",
      "url": "https://learn.microsoft.com/api/mcp"
    }
  ]
}
```

名称区分大小写，支持 1–64 位字母、数字、下划线和短横线，且以字母或数字开头。简介可为空。URL 必须为 HTTP 或 HTTPS，不能包含内嵌账号和片段。当前不要把凭据放进地址，服务详情会展示完整 URL。

## 操作

| 命令 | 作用 |
| --- | --- |
| `/mcp` | 打开服务管理列表 |
| `/mcp add` | 打开添加表单 |
| `/mcp edit <名称>` | 编辑名称、简介和地址 |
| `/mcp show <名称>` | 交互模式获取远程信息和工具参数；管道输出本地配置 |
| `/mcp search <关键词>` | 在本地名称、简介和地址中搜索 |
| `/mcp add <名称> <URL>` | 直接添加，也支持管道 |
| `/mcp edit <名称> <URL>` | 直接修改地址，保留简介，也支持管道 |
| `/mcp remove <名称>` | 直接删除指定配置，也支持管道 |

列表：↑↓ 选择、Enter 详情、`a` 添加、`e` 编辑、`d` 删除确认、`/` 输入本地搜索词。详情用 ↑↓ / PgUp / PgDown 查看长地址和完整工具参数，`r` 重新获取。Esc 从详情或确认页返回列表，再按 Esc 返回聊天。

表单：Tab / Shift+Tab / ↑↓ 切换字段，Enter 进入下一项；选中“保存配置”后 Enter 才保存。Esc 放弃草稿。保存失败会带着原输入重新显示表单。

管道中的 `/mcp`、`show` 和 `search` 输出文本；不带完整参数的 `add` 和 `edit` 会提示使用交互终端。管理命令不写入模型历史，不改变 `/tools` 的基础工具列表。

## 阅读顺序

```text
App.New
  → config.Load：取得 mcpConfigPath
  → newApp → mcp.Open：加载共享 Manager

App.Run
  → command.Lookup
  → handleMCP：读参数或菜单选择
  → Manager.Add / Update / Remove
  → save：写同目录临时文件、Sync、Close、Rename
  → 保存成功才更新 Manager 内存
```

- `server.go`：服务字段和格式校验。
- `client.go`：共用连接入口、详情发现、工具调用和关闭。
- `../../tools/mcp/tool.go`：将远程工具绑定为统一接口并管理本项连接。
- `manager.go`：读取列表、搜索与增删改查。
- `storage.go`：版本检查和文件读写。
- `../command/mcp.go`：顺序组织菜单和持久化操作。
- `../../views/commands/mcp.go`、`mcp_form.go`：显示与草稿，不持有 Manager。

Manager 为后台任务提供带锁的配置快照，修改事务先保存文件，再更新内存；读取锁不覆盖文件或网络等待。仍不支持多个 CLI 进程同时编辑同一文件，也不会自动合并运行期间的外部文件修改。

## 在问题中选择 MCP

输入 `/` 可以看到命令、技能和已配置的 MCP 候选；输入 `/mcp{` 或服务名称可以筛选。Enter 只确认候选，插入绿色 `◇ 服务名` 标签，再按 Enter 才提交问题。标签可整体移动、删除和复制，也支持直接输入或粘贴：

```text
请使用 /mcp{microsoft-learn} 查询 Go 在 Azure 上的部署方法
```

引用存为 `query.MCP` 片段，任务入队时复制；到执行时才读取所选服务的配置并获取工具。代码围栏、行内代码和 URL 中的同样文字不会激活 MCP。未知服务或连接失败会在请求模型前报错；同一服务的重复引用只连接一次。

```text
任务开始
  → Input.MCPNames：当前输入中的服务名
  → Manager.Select：读取本项配置快照
  → tools/mcp.Prepare：连接、分页发现、绑定远程执行入口
  → Registry.WithRemote：创建本项独立工具集合
  → 每轮请求发送该集合的 ModelTools
  → tool_calls 交给同一集合 Execute
  → MCP tools/call，真实结果写入 role=tool 历史
  → 任务结束，关闭本项连接
```

同一任务的多轮请求复用连接和工具集合。每个任务拥有自己的连接，取消一个任务不会关闭其他任务的连接。当前不做跨任务连接池或工具列表缓存。

下一条问题没有 MCP 引用时，只发送基础工具声明，也不能调用上次任务的 MCP 工具。过去的工具调用与返回值仍保留在聊天历史；历史不决定当前工具权限。切换或恢复 Session 不会自动沿用 MCP 选择。

远程工具名映射为不超过 64 字符的稳定别名（含服务、原名和摘要），避免服务间重名。完整 inputSchema 原样传给模型，复杂业务参数约束由远端校验；本地仍校验参数是 JSON 对象。Agent 通过同一注册表调用，不按 MCP/内置/外部工具写分支。

MCP 的 `content`、`structuredContent` 和 `isError` 保存在统一结果的 `data` 中，`isError=true` 会转为 `ok=false` 并进入现有重试流程。当前结果作为 JSON 文本交回模型，不做图片/音频的多模态转换。服务说明只在详情展示，不提升为系统指令。

单次 HTTP 请求超时为 30 秒，详情整体发现也限时 30 秒；工具列表最多获取 100 页，重复分页 cursor 明确报错。

## 验证

```sh
go test ./internal/mcp ./internal/command ./internal/app ./config ./views/commands
```

默认测试使用临时文件和本地模拟模型/MCP 服务，不请求付费模型或公网。覆盖配置重载、重复名称、非法地址、损坏文件、写入失败不改变内存、菜单与表单 Esc、搜索、删除确认、中文窄窗口和详情滚动。


真实微软地址检查需显式启用，只获取信息和工具列表：

```sh
MINI_GNET_LIVE_MCP=1 go test ./internal/mcp -run TestMicrosoftLearnDiscovery -v -count=1
```
