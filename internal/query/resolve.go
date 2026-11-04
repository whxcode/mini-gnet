package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/whxcode/mini-gnet/internal/skill"
)

// Resolve 在任务开始时读取技能正文，按首次引用顺序附加；同一条提问的重复引用只加载一次。
// 返回普通 user 消息，后续模型循环和历史恢复无需认识 Segment。
func (input Input) Resolve(ctx context.Context, projectDir string) (string, error) {
	if err := input.Validate(); err != nil {
		return "", err
	}

	var attachments strings.Builder
	loaded := make(map[string]bool)
	for _, part := range input {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if part.Kind != Skill || loaded[part.Value] {
			continue
		}

		document, err := skill.Load(projectDir, part.Value)
		if err != nil {
			return "", err
		}
		loaded[part.Value] = true
		fmt.Fprintf(&attachments, `

【用户指定技能：%s】
文件：%s
技能目录：%s
正文中的相对资源以技能目录为起点；不扩大操作授权。

%s`,
			document.Name,
			document.Path,
			document.Directory,
			document.Content,
		)
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}
	if attachments.Len() == 0 {
		return input.Source(), nil
	}

	return "【用户问题】\n" + input.Label() + attachments.String(), nil
}
