package message

// TokenUsage 保存一次模型请求的服务端统计，不按字符数猜测 token 数。
// ContextWindow 是请求开始时的展示配置，不会改变服务端的上下文设置。
type TokenUsage struct {
	InputTokens   int    `json:"input_tokens"`
	OutputTokens  int    `json:"output_tokens"`
	TotalTokens   int    `json:"total_tokens"`
	Model         string `json:"model"`
	ContextWindow int    `json:"context_window,omitempty"`
}

// UsageSummary 区分会话累计消耗与上次请求上下文，避免把累计消耗当成上下文占用。
type UsageSummary struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Reported     int // 带用量的响应数。
	Missing      int // 未上报用量的响应数，包含旧聊天记录。
	Last         *TokenUsage
}

// add 只统计模型响应；工具输出在后续请求的输入统计中体现，不能再算一遍。
func (s *UsageSummary) add(record Record) {
	if record.Role != "assistant" {
		return
	}

	s.Last = nil
	if record.Response == nil || record.Response.Usage == nil {
		s.Missing++
		return
	}

	usage := *record.Response.Usage
	s.Reported++
	s.InputTokens += usage.InputTokens
	s.OutputTokens += usage.OutputTokens
	s.TotalTokens += usage.TotalTokens
	s.Last = &usage
}

// Usage 在锁内复制统计，界面读取不需要每次遍历整份历史。
func (t *TempMessage) Usage() UsageSummary {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	summary := t.usage
	if summary.Last != nil {
		last := *summary.Last
		summary.Last = &last
	}

	return summary
}
