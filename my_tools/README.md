# 用户工具目录

每个子目录用一个 `tool.json` 描述工具、输入输出和启动方式。应用启动时加载这些定义，统一显示在 `/tools` 中；增加工具不需要修改 Go 代码，重启程序即可。

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
