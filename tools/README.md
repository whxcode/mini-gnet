# 工具定义、注册与调用

启动时注册 `run_bash`、`run_python`，再加载配置目录中的外部工具。所有工具都实现 `Tool`，Agent 只按名称调用注册表，不按工具来源或名称写执行分支。

```text
tools/
  tool.go             Definition、Tool、Request、Result 和错误代码
  schema.go           本版支持的 schema 解析和校验
  registry.go         查找、校验、调用、统一返回
  format.go           工具参数和结果的显示，兼容旧历史
  builtin/register.go 程序自带工具的唯一注册位置
  external/
    load.go           读取 tool.json，解析启动配置
    process.go        通用外部程序执行器
    output.go         限制进程输出大小
  bash/
    schema.json       run_bash 定义
    tool.go           参数与调用接口
    bash.go           Bash 执行实现
  run/
    schema.json       run_python 定义
    tool.go           参数与调用接口
    python.go         临时脚本执行与清理
```

内置工具的 `schema.json` 使用 `go:embed` 编译进程序，修改内置定义需要重新编译。外部工具的 `tool.json` 在启动时从磁盘读取，添加或修改后重启即可，不需要重新编译。

## 阅读顺序

1. `bash/schema.json`：看工具名、说明、输入参数和输出数据结构。
2. `tool.go`：看统一接口和请求、响应。
3. `bash/tool.go`：看一个工具如何实现接口、解析自己的参数。
4. `internal/app/app.go` 的 `New`：先 `builtin.Load()`，再 `external.LoadDir(cfg.ToolsDir)`，最后一起交给 `tools.NewRegistry()`。
5. `registry.go` 的 `Execute`：按顺序查找、检查参数、执行、检查输出和返回。
6. `internal/agent/agent.go`：模型声明来自 `ModelTools()`，模型调用交给同一注册表的 `Execute()`。

注册表启动时构造，运行期间只读，App 和 Agent 共用一份。`Definitions()` 和 `ModelTools()` 返回副本，菜单展示不会更改工具定义。每个会话和任务不复制工具实现。

## schema 约定

外层字段为 `schema_version`（目前为 1）、`name`、`description`、`input_schema`、`output_schema`。工具名称唯一，允许字母、数字、下划线和连字符，最长 64 个字符。

`input_schema` 校验模型 arguments；`output_schema` 校验返回结果里的 data，不包含 ok/error 外层。当前实现是内置工具需要的 JSON Schema 子集：

- 根节点必须为 object；支持 object、string 和 64 位 integer。
- 支持 properties、required、additionalProperties、description、minLength。
- 定义文件中应显式填写 additionalProperties；本版未填写时按 false 处理。
- 未支持的类型或关键字在注册时报错，避免声明约束却不执行校验。

错误 schema、重复工具名会导致初始化失败，不向模型公布不能正常注册的工具。

## 手动调用，不需要模型

```go
registry, err := builtin.NewRegistry()
if err != nil {
    return err
}

result := registry.Execute(ctx, "run_bash", tools.Request{
    Arguments: json.RawMessage(`{"command":"cat /etc/hosts"}`),
    Context: tools.Context{WorkDir: "/Users/mac/study/mini-gnet"},
})
data, err := json.MarshalIndent(result, "", "  ")
if err != nil {
    return err
}
fmt.Println(string(data))
```

模型只提供 arguments。程序提供的 WorkDir 是默认位置，不是访问范围；读取 `/etc/hosts` 不需要改变 cwd。能否访问文件由操作系统权限决定。

`run_bash` 参数：

```json
{"command":"cat /etc/hosts"}
```

```json
{"command":"pwd","cwd":".."}
```

cwd 省略时使用默认目录；绝对路径直接使用，相对路径基于默认目录解析，只设置本次子进程的 `cmd.Dir`，不使用 `os.Chdir`。`pwd` 不接受目录参数。Bash 使用 `-c`，不加载可能改变执行目录的登录脚本；保留原来的命令白名单和单行语法限制。白名单本身不是只读权限沙箱。

`run_python` 参数：

```json
{"code":"print(1 + 2)"}
```

Python 保留临时目录语义：创建 main.py、执行、清理。访问项目或其他文件时，代码可使用绝对路径。运行环境仍需要 Bash / python3；嵌入的是工具定义，不是解释器。

## 统一输出

```json
{
  "ok": true,
  "data": {"stdout":"3\n","stderr":"","exit_code":0},
  "error": null
}
```

```json
{
  "ok": false,
  "data": {"stdout":"partial\n","stderr":"RuntimeError: failed\n","exit_code":1},
  "error": {"code":"EXECUTION_FAILED","message":"exit status 1"}
}
```

工具实现填写 `Result.Data` 并返回 Go error，注册表填写统一的 ok/error。参数或查找失败时 data 为 null；执行失败时保留有效的部分输出。stderr 有内容不一定表示失败。进程没有启动或被信号结束时，exit_code 可能为 -1。

标准错误码：TOOL_NOT_FOUND、INVALID_ARGUMENTS、EXECUTION_FAILED、TIMEOUT、CANCELED、INVALID_OUTPUT、INTERNAL_ERROR。注册表给单次调用设置最多五分钟的超时，父任务取消或更早的超时同样有效。

Agent 将同一 JSON 返回模型并保存到历史。错误提示和最多三次工具失败的限制对工具统一处理，不再单独判断 Python 名称；模型 stop 且无工具调用时仍按协议结束本项。

## 查看工具

- `/tools`：查看名称、说明、参数类型和必填/可选状态。
- `/tools run_bash`：查看该工具完整定义，包括输出 schema。
- 交互菜单支持滚轮、上下键、PgUp/PgDown、Home/End；Esc 或 Enter 返回。
- 管道模式返回相同的文本说明；命令不执行工具，不写入模型对话。

列表和模型拿到的工具声明来自同一注册表；默认目录中的天气示例会显示为 `city_weather`。

## 调用详情

按 `Ctrl+O` 查看完整执行记录。每次调用显示 `tool_name`、模型返回的 `tool_call_id` 和完整的原始 `arguments` 字符串；不再把 `{"city":"成都"}` 简化成“成都”，也不重新排版或解码其中的转义序列。

工具返回后，同一条调用记录补充实际进程信息：

- `command`：解析后的程序路径和参数组成的 JSON 数组。
- `argv`：传给进程的完整参数数组，包含 argv[0]；数组能明确区分空格、引号与参数边界。
- `cwd`：本次进程的工作目录；临时 Python 会记录当时真正创建的目录。
- `started`：是否成功创建了进程。
- `exit_code`：可取得的进程退出码；被信号终止时可能为 -1。
- `duration_ms`：这次启动和执行花费的毫秒数。

结果记录也显示 `tool_call_id`，便于匹配多个工具或重试。新执行信息保存在聊天 JSONL 的本地 `tool_execution` 字段，恢复会话时使用当时的记录；不加入模型上下文，也不改变 `ok / data / error` 协议。

旧历史仍能显示原始参数与调用编号，但没有记录的命令路径、目录和耗时不会重新推算。参数校验失败、工具不存在等未进入执行器的调用，也不会伪造进程信息。

## 外部工具目录

配置字段是 `toolsDir`，默认值是 `my_tools`。相对路径以配置的 `projectDir` 为起点；未指定项目目录时以启动目录为起点。也支持绝对路径和 `~/`。旧配置没有这个字段时使用默认值。

```json
{
  "toolsDir": "my_tools"
}
```

上面只是需要新增的字段，不要替换完整配置。也可以在 `/config` 编辑“工具目录”。工具在启动时加载；修改路径、增加或删除工具后需要重启，运行中的注册表不改变。

```text
my_tools/
  weather/
    tool.json      工具定义和启动方式
    main.py        Python 天气查询示例
```

程序只扫描直接子目录的 `tool.json`，按目录名排序。工具目录不存在时只加载内置工具；没有 `tool.json` 的子目录会跳过。损坏定义、无效执行目录或重复工具名会阻止启动，并返回具体错误；外部工具不能覆盖内置工具。

`tool.json` 在公共定义之外增加 `execution`：

```json
"execution": {
  "command": "python3",
  "args": ["main.py"],
  "cwd": "."
}
```

- 裸 `command`（如 `python3`、`node`）由 PATH 查找；也可以填写可执行文件的绝对路径。
- 带路径的相对命令（如 `./bin/tool`）相对于 `tool.json` 所在目录解析。
- `cwd` 省略或为 `.` 时使用工具目录，也可以填写 `..` 或其他绝对路径；它不是文件访问边界。
- `args` 原样交给进程；脚本等相对参数由被启动的程序按 cwd 解释。不会进行 Shell 展开、变量替换或自动安装依赖。
- `execution` 不发送给模型，不出现在公共工具定义中。

外部程序每次调用启动一次，协议为：

| 通道 | 约定 |
| --- | --- |
| stdin | 一个 JSON 参数对象；写完后关闭输入，脚本可以直接读取到 EOF |
| stdout | 成功时输出一个符合 output_schema 的 JSON 对象，不加代码围栏或日志 |
| stderr | 日志和失败原因；退出码为 0 时日志不影响成功结果 |
| 退出码 | 0 成功，非 0 失败 |

执行器把 stdout 的对象放入 `Result.Data`，注册表统一生成 `ok / data / error`。外部程序不用自己包装这三项。失败退出时 data 为 null，错误包含退出原因和 stderr；无效 JSON 或不符合 schema 的输出返回 `INVALID_OUTPUT`。

单次调用使用注册表的五分钟上限，父任务取消或超时同样生效。stdout 最多 1 MiB、stderr 最多 64 KiB，超限会停止子进程并返回 `INVALID_OUTPUT`。启动配置和脚本需要来自你信任的来源；外部程序使用本机权限执行，不是沙箱。

## 天气示例

`my_tools/weather/main.py` 使用 Python 3 标准库访问 Open-Meteo 的地理编码和天气接口，不需要 API Key 或 pip 安装。需要能联网；地点取首个匹配项，结果附地区和国家以便核对。找不到中文城市名时可以尝试英文名。

启动应用后输入：

```text
/tools city_weather
请使用 city_weather 查询成都当前天气。
```

也可以从项目根目录手动测试脚本，不经过模型：

```sh
printf '%s\n' '{"city":"成都"}' | python3 my_tools/weather/main.py
```

查看不经过模型的 Go 调用示例：[my_tools/README.md](../my_tools/README.md)。

## 验证

```sh
go test -race ./tools/... ./internal/... ./views/... ./net -timeout 45s
go test ./config -run TestToolsDirectory
python3 -B -m unittest discover -s my_tools/weather -v
```

测试使用临时目录和本地模拟模型，覆盖项目外加载、绝对路径、上层 cwd、外部进程输入输出、错误结果、取消、输出上限、多轮调用和 /tools 菜单。天气测试使用固定响应，不请求公网；所有自动测试都不请求付费模型。
