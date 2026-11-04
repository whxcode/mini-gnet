# 用户工具目录

每个子目录用一个 `tool.json` 描述工具、输入输出和启动方式。应用启动时加载这些定义，统一显示在 `/tools` 中；增加工具不需要修改 Go 代码，重启程序即可。

| 工具 | 实现 | 用途 |
| --- | --- | --- |
| `city_weather` | Python 3 标准库 | 查询城市天气，需要联网 |
| `web_search` | Node.js 22+ 内置 fetch | 联网搜索网页，返回标题、链接和摘要 |

两个工具都不需要安装第三方依赖。

天气示例的阅读顺序：

1. `weather/tool.json`：名称 `city_weather`、必填参数 `city`、输出字段，以及 `python3 main.py` 启动方式。
2. `weather/main.py`：从 stdin 读取 JSON，查城市坐标、查天气，再向 stdout 输出 JSON。
3. `tools/external/process.go`：Go 如何启动这个脚本、传入参数并收集结果。
4. `internal/app/app.go`：`New` 如何把内置工具和目录里的工具一起注册。

天气接口为 Open-Meteo，示例无需密钥，只需要 Python 3 和网络。返回的数据时间来自接口；同名城市取首个匹配，结果中的地区和国家用于确认位置。

## 手动运行 Python

从项目根目录运行：

```sh
printf '%s\n' '{"city":"成都"}' | python3 my_tools/weather/main.py
```

失败原因写入 stderr，并返回非零退出码。成功时 stdout 只有一个 JSON 对象。

## Node.js 联网搜索工具

文件是 `web_search/tool.json` 和 `web_search/main.mjs`。重启应用后输入：

```text
/tools web_search
请使用 web_search 搜索 Go context 取消机制，返回 5 条结果并附链接。
```

也可以直接调用，不经过模型：

```sh
printf '%s\n' '{"query":"Go language","limit":3}' | node my_tools/web_search/main.mjs
```

`query` 必填，支持中文和英文；`limit` 可选，默认 5，范围 1–10。返回的结构是：

```json
{
  "query": "搜索词",
  "source": "Bing RSS",
  "search_url": "本次搜索入口地址",
  "count": 1,
  "results": [
    {
      "title": "页面标题",
      "url": "页面链接",
      "snippet": "搜索引擎提供的摘要"
    }
  ]
}
```

这个示例使用必应公开 RSS 搜索入口，不需要 API Key，适合个人功能测试。结果排序和相关性由搜索源决定；摘要不等于已经读取、核实网页全文。Bing RSS 对结果的使用有限制，正式产品应选择符合用途的搜索 API。

异步发生在 Node.js 内部：等待 `fetch()` 和响应正文读取完成，stdout 才输出一个 JSON 对象。Go 等待子进程结束后校验结果，无需增加 Node.js 专用执行分支。

请求最长等待 15 秒，响应上限 2 MiB。搜索源返回 HTTP 错误、验证码页面或不完整 RSS 时，脚本通过 stderr 报错并以非零状态退出；合法的空 RSS 则返回空 results。解析器只处理此搜索源的固定 RSS 格式，不是通用 XML 解析器。

离线测试使用固定响应，不访问公网或模型：

```sh
node --test my_tools/web_search/main.test.mjs
go test -v ./tools/external -run TestWebSearchNodeProtocol -timeout 45s
```

按 `Ctrl+O` 可以查看实际的 Node 命令、原始搜索参数、调用 ID、目录和耗时。

## 不接模型，用 Go 调用

下面的 `main` 可以单独保存为临时 Go 文件，再从项目根目录 `go run /绝对路径/demo.go`。目录扫描仍以当前项目为起点。

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    "github.com/whxcode/mini-gnet/tools"
    "github.com/whxcode/mini-gnet/tools/external"
)

func main() {
    implementations, err := external.LoadDir("my_tools")
    if err != nil {
        log.Fatal(err)
    }

    registry, err := tools.NewRegistry(implementations...)
    if err != nil {
        log.Fatal(err)
    }

    result := registry.Execute(context.Background(), "city_weather", tools.Request{
        Arguments: json.RawMessage(`{"city":"成都"}`),
    })

    output, err := json.MarshalIndent(result, "", "  ")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(string(output))
}
```

完整路径和协议约定见 [tools/README.md](../tools/README.md)。
