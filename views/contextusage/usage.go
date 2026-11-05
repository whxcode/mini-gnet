// Package contextusage 为会话卡片和详情提供统一的上下文统计展示，不修改历史。
package contextusage

import (
	"fmt"
	"strings"

	"github.com/whxcode/mini-gnet/views/message"
)

// Compact 根据数量选择单位，并避免四舍五入后出现 1000.0k。
func Compact(count int) string {
	switch {
	case count >= 999_950:
		return fmt.Sprintf("%.1fM", float64(count)/1_000_000)
	case count >= 1_000:
		return fmt.Sprintf("%.1fk", float64(count)/1_000)
	default:
		return fmt.Sprintf("%d", count)
	}
}

// UsedText 只使用本会话最近响应的输入加输出估算占用，不使用累计消耗。
func UsedText(usage message.UsageSummary) string {
	if usage.Last == nil {
		if usage.Missing > 0 {
			return "已用 -- · 未上报"
		}

		return "已用 -- · 尚无数据"
	}

	return "已用 ≈" + Compact(usage.Last.InputTokens+usage.Last.OutputTokens) + " tokens"
}

// RemainingText 缺少用量或上限时保持未知，超过上限时不显示负数。
func RemainingText(usage message.UsageSummary) string {
	if usage.Last == nil {
		return "剩余 --"
	}

	if usage.Last.ContextWindow <= 0 {
		return "剩余 -- · 上限未设置"
	}

	remaining := usage.Last.ContextWindow - usage.Last.InputTokens - usage.Last.OutputTokens
	if remaining < 0 {
		return "剩余 0 · 已超上限"
	}

	return "剩余 ≈" + Compact(remaining) + " tokens"
}

// Summary 用于当前会话标题，详情入口与卡片使用相同统计口径。
func Summary(usage message.UsageSummary) string {
	return "上下文 · " + UsedText(usage) + " / " + RemainingText(usage) + " · /context"
}

// Detail 生成打开菜单时的会话快照；累计消耗与上下文占用分区展示。
func Detail(id, title, status string, usage message.UsageSummary) string {
	lines := []string{
		"会话：" + title,
		"ID：" + id,
		"状态：" + status + "（打开菜单时的快照）",
		"",
		"上下文占用 · 最近一轮估算",
		UsedText(usage),
		RemainingText(usage),
	}

	if last := usage.Last; last != nil {
		used := last.InputTokens + last.OutputTokens
		lines = append(lines,
			fmt.Sprintf("已用明细：输入 %d + 输出 %d = %d tokens", last.InputTokens, last.OutputTokens, used),
			"该轮模型："+last.Model,
		)

		if last.ContextWindow > 0 {
			percent := float64(used) / float64(last.ContextWindow) * 100
			lines = append(lines,
				fmt.Sprintf("该轮配置上限：%s（%d tokens）", Compact(last.ContextWindow), last.ContextWindow),
				fmt.Sprintf("剩余估算：%d tokens · 占用 %.1f%%", max(0, last.ContextWindow-used), percent),
			)
		} else {
			lines = append(lines, "该轮未设置上下文上限。请在 /config 填写服务实际使用的窗口大小，下一项任务生效。")
		}
	} else if usage.Missing > 0 {
		lines = append(lines, "最近响应未包含用量（旧记录也可能缺失），不能沿用更早的统计。")
	} else {
		lines = append(lines, "尚无模型响应的用量数据；未知不等于 0，系统提示词也会占用上下文。")
	}

	lines = append(lines,
		"",
		"本会话累计消耗 · 不代表上下文占用",
		fmt.Sprintf("已记录输入：%s（%d tokens）", Compact(usage.InputTokens), usage.InputTokens),
		fmt.Sprintf("已记录输出：%s（%d tokens）", Compact(usage.OutputTokens), usage.OutputTokens),
		fmt.Sprintf("已记录合计：%s（%d tokens）", Compact(usage.TotalTokens), usage.TotalTokens),
		fmt.Sprintf("已上报 %d 轮 · 缺失 %d 轮（含旧记录）", usage.Reported, usage.Missing),
		"",
		"统计口径",
		"仅统计当前 Session，不累加其他会话。每轮模型响应结束后更新，包含工具调用的中间轮次。",
		"流式输出、新提问和尚未发送的工具结果可能尚未计入；推理 token 和服务端截断也可能导致估算与下一次请求不同。",
		"上限取自该轮请求的配置，仅用于展示，不会修改模型服务设置，也未预留下一次回答的输出空间。",
		"接口未提供系统提示词、历史、技能和工具定义的独立 token 明细，因此不编造分类占比。",
	)

	return strings.Join(lines, "\n")
}
