package agent

// Role 标明消息来源，不包含显示颜色或图标。
type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
	Tool      Role = "tool"
	System    Role = "system"
	Error     Role = "error"
)

// Kind 区分完整消息、执行进度与流式文字。
type Kind int

const (
	Message Kind = iota // 完整消息。
	Status              // 执行步骤或进度。
	Delta               // 模型新生成的文本片段。
)

// Event 是 Agent 输出的数据，渲染方式由界面决定。
type Event struct {
	Kind  Kind   // 完整消息、执行进度或文字片段。
	Role  Role   // 消息来源。
	Name  string // 用户名、模型名或工具名。
	Text  string // 原始消息文字，由显示层负责转义和排版。
	Stage Stage  // 中间过程或最终回答；普通提示可不设置。
}

// Stage 区分一项用户提问内的中间轮次与最终回答，不根据正文内容猜测。
type Stage string

const (
	Process Stage = "process" // 继续调用模型或工具的中间输出。
	Final   Stage = "final"   // 收到 stop 且无工具调用后的最终回答。
)
