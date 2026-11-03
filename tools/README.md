# 工具定义、注册与调用

当前注册 `run_bash`、`run_python`。两个工具都实现 `Tool`，Agent 只按名称调用注册表，不按工具来源或名称写执行分支。

```text
tools/
  tool.go             Definition、Tool、Request、Result 和错误代码
  schema.go           本版支持的 schema 解析和校验
  registry.go         查找、校验、调用、统一返回
  format.go           工具参数和结果的显示，兼容旧历史
  builtin/register.go 程序自带工具的唯一注册位置
  bash/
    schema.json       run_bash 定义
    tool.go           参数与调用接口
    bash.go           Bash 执行实现
  run/
    schema.json       run_python 定义
    tool.go           参数与调用接口
    python.go         临时脚本执行与清理
```

每份 `schema.json` 使用 `go:embed` 编译进对应工具。启动时解析嵌入内容，不读取当前目录中的 JSON；原根目录 `tools.json` 和单独的 `tools/list` 加载器已移除。修改内置定义需要重新编译。

## 阅读顺序

1. `bash/schema.json`：看工具名、说明、输入参数和输出数据结构。
2. `tool.go`：看统一接口和请求、响应。
3. `bash/tool.go`：看一个工具如何实现接口、解析自己的参数。
4. `builtin/register.go`：看如何把两个工具交给 `NewRegistry`。
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

列表和模型拿到的工具声明来自同一注册表。当前没有外部工具目录扫描或动态加载。

## 验证

```sh
go test -race ./tools/... ./internal/... ./views/... ./net -timeout 45s
```

测试使用临时目录和本地模拟模型，覆盖项目外加载、绝对路径、上层 cwd、错误结果、输出校验、多轮调用和 /tools 菜单；不会请求付费模型。
