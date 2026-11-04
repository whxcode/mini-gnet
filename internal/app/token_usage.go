package app

import (
	"fmt"

	"github.com/whxcode/mini-gnet/views/message"
)

// tokenTotalText 标明已记录的消耗；旧记录或接口未上报时，不把未知部分算作零。
func tokenTotalText(usage message.UsageSummary) string {
	if usage.Reported == 0 && usage.Missing > 0 {
		return "Token -- 未上报"
	}

	prefix := "Token 已记录 "
	if usage.Missing > 0 {
		prefix = "Token 已知 "
	}

	return prefix + compactTokens(usage.TotalTokens)
}

// tokenContextText 用最近一轮输入加输出估算上下文占用，不使用会话累计 token。
// 这是上次请求的快照，不包含刚输入的问题或尚未发给模型的工具结果。
func tokenContextText(usage message.UsageSummary) string {
	if usage.Last == nil {
		return "上次上下文 --"
	}

	if usage.Last.ContextWindow <= 0 {
		return "上次上下文 --"
	}

	used := usage.Last.InputTokens + usage.Last.OutputTokens
	percent := float64(used) / float64(usage.Last.ContextWindow) * 100
	return fmt.Sprintf("上次上下文 ≈%.1f%%", percent)
}

// tokenContextCountText 在百分比下面列出分子、分母或缺失原因。
func tokenContextCountText(usage message.UsageSummary) string {
	if usage.Last == nil {
		return "尚无用量数据"
	}

	if usage.Last.ContextWindow <= 0 {
		return "上限未设置 · /config"
	}

	used := usage.Last.InputTokens + usage.Last.OutputTokens
	return compactTokens(used) + " / " + compactTokens(usage.Last.ContextWindow) + " tokens"
}

// compactTokens 缩短卡片里的计数；小于一千时显示整数。
func compactTokens(count int) string {
	if count >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(count)/1_000_000)
	}

	if count >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(count)/1_000)
	}

	return fmt.Sprintf("%d", count)
}

// tokenDetailText 在当前会话标题下显示精确计数和比例口径，单会话也能查看。
func tokenDetailText(usage message.UsageSummary) string {
	if usage.Reported == 0 {
		return tokenContextText(usage) + " · " + tokenTotalText(usage)
	}

	text := tokenContextText(usage)
	if usage.Last != nil && usage.Last.ContextWindow > 0 {
		text += fmt.Sprintf(" · %d/%d tokens",
			usage.Last.InputTokens+usage.Last.OutputTokens,
			usage.Last.ContextWindow,
		)
	} else {
		text += " · " + tokenContextCountText(usage)
	}
	text += fmt.Sprintf(" · 已记录 %d（输入 %d / 输出 %d）", usage.TotalTokens, usage.InputTokens, usage.OutputTokens)

	if usage.Missing > 0 {
		text += " · 部分用量未知"
	}

	return text
}
