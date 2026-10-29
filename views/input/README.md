# 终端输入组件

交互终端由 `internal/app/terminal.go` 管理工作区，每个会话使用 `NewTextArea()` 创建自己的输入框，任务执行期间仍可输入和切换。
管道输入由 `internal/app/pipe.go` 单独按行读取；本目录只负责终端文本编辑，不判断运行模式。

输入区初始只有一行。`DynamicHeight` 会按实际显示行数自动撑开，包含手动换行和
长文本折行；删除内容或把窗口拉宽后会相应收缩，不使用固定三行的内部滚动区域。多会话界面会将输入区限制在实际终端高度内，超长文本仍可移动光标编辑。

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
| F2 | 已打开会话列表 |
| Alt+← / Alt+→ | 切换前一个 / 后一个会话 |
| Alt+N | 新建会话，保留旧会话草稿 |
| Ctrl+C | 当前任务执行中时停止它，否则请求退出 |

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
也可以直接使用 Ctrl+J 换行。多会话输入框支持这些组合键；终端无法区分 Shift+Enter 时使用 Ctrl+J。

## 阅读代码顺序

1. `NewTextArea`：创建输入框、设置尺寸和换行快捷键。
2. `internal/app/terminal.go`：会话工作区的提交、切换和显示逻辑。
3. `Update`：先处理发送与退出，再把普通编辑事件交给 textarea。
4. `View`：显示正文、光标和快捷键提示。
5. `internal/app/app.go`：取得输入后分流命令和任务。

当前仅处理文本，未加入图片、文件补全或 skills 菜单。
