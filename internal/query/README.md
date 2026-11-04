# 结构化提问

输入采用统一的 `kind/value`：

```json
[
  {"kind":"text","value":"请使用 "},
  {"kind":"skill","value":"code-review"},
  {"kind":"text","value":" 检查当前项目"}
]
```

当前只支持 `text`、`skill`，没有实现文件或工具引用。

## 执行顺序

1. 输入框保存普通文字与技能标签，候选只读取技能摘要。
2. `Composer.Submission()` 检查手输语法，交出独立片段。管道入口使用 `query.Parse` 转换 `/skills{名称}`。
3. `Session.EnqueueInput` 校验片段，`Task.NewInput` 再复制一份，保存到队列；不读取正文。
4. `Agent.Run` 在任务开始时调用 `Input.Resolve`，读取当前工作目录中的技能正文。同一技能重复引用只附加一次。
5. 组装成普通 user 消息，保存历史；后面的多轮模型请求复用这条消息，不再解析或加载手动引用。

排队期间文件改变，使用开始执行时的版本。读取失败结束本任务并显示错误，不请求模型；后续排队任务可以继续执行。

## 编辑和展示

`Source()` 是可复制的语法，`Label()` 是不含正文的简短显示文字；两者都不能替代模型真正需要的正文。

编辑器使用 Bubbles 管理文字、选区和删除。每个标签在编辑缓冲区里只占一个内部字符，渲染时替换为完整名称，左右键和 Backspace/Delete 因此不会切开标签。提交、复制和保存时都转换成真实内容，内部字符不进入 JSONL。

`user_input` 只保存原始片段，供会话标题和恢复界面使用；发送给模型时只取正常的 role/content。旧历史没有该字段时，继续显示原来的 content。

## 文件职责

- `input.go`：片段结构、语法解析、校验和简短显示。
- `resolve.go`：任务开始时加载技能并组装消息。
- `views/input/composer.go`：编辑和结构化快照。
- `views/input/composer_layout.go`：标签颜色、折行、选区和光标位置。
- `internal/app/completion.go`：候选命令与技能，确认候选不发送任务。
