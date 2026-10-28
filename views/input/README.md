# 终端输入组件

`internal/app/app.go` 的 `New` 创建一次 `input.NewReader()`，每轮调用 `Read()` 获取完整问题。
交互终端使用 Bubbles textarea；管道输入仍然每行作为一条问题。

输入区初始只有一行。`DynamicHeight` 会按实际显示行数自动撑开，包含手动换行和
长文本折行；删除内容或把窗口拉宽后会相应收缩，不使用固定三行的内部滚动区域。

## 常用操作

| 按键 | 操作 |
| --- | --- |
| Enter | 发送非空问题 |
| Shift+Enter | 插入换行，需要终端上报修饰键 |
| Ctrl+J / Alt+Enter | 备用换行 |
| 方向键 | 移动光标；上下键在多行之间移动 |
| Backspace / Delete | 删除光标前 / 后的字符 |
| Home / End，Ctrl+A / Ctrl+E | 移到行首 / 行尾 |
| Ctrl+U / Ctrl+K | 删除光标到行首 / 行尾的内容 |
| Shift+方向键 | 选择文本，需要终端上报该组合键 |
| Ctrl+C | 退出输入和程序 |

终端的常规粘贴快捷键可以粘贴多行文本。Bubble Tea 自动开启 bracketed paste；
支持该协议的终端把粘贴内容作为 `PasteMsg`，内容里的换行不会触发发送。

## Shift+Enter 的终端兼容性

Bubble Tea v2 默认请求增强键盘协议。在支持的终端中，程序能区分 Enter 与
Shift+Enter。如果终端对两者都发送同一个回车字符，程序无法判断是否按住 Shift。

如果使用 iTerm2 且 Shift+Enter 仍然直接发送，可在
Settings → Profiles → Keys → Key Mappings 中添加：

- 快捷键：Shift+Enter
- Action：Send Escape Sequence
- 内容：`[13;2u`（不需要再输入 ESC 字符）

这会发送可被 Bubble Tea 识别的 Shift+Enter 序列。项目不会自动修改终端设置。
也可以直接使用 Ctrl+J 换行。没有检测到增强协议时，输入区会显示兼容提示；
手动配置了按键映射后，即使提示仍显示，Shift+Enter 也可以工作。

## 阅读代码顺序

1. `newEditor`：创建输入框、设置尺寸和换行快捷键。
2. `Update`：先处理发送与退出，再把普通编辑事件交给 textarea。
3. `View`：显示正文、光标和快捷键提示。
4. `Reader.Read`：运行一次编辑，恢复终端后将问题交回主循环。

当前仅处理文本，未加入图片、文件补全或 skills 菜单。
