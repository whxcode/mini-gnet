package session

import (
	"fmt"

	"github.com/whxcode/mini-gnet/internal/skill"
)

// SelectSkill 保存下一次提问要附加的技能副本；nil 仅取消待选状态，不删除历史消息。
func (s *Session) SelectSkill(document *skill.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("当前会话已关闭")
	}

	if document == nil {
		s.selectedSkill = nil

		return nil
	}

	selected := *document
	s.selectedSkill = &selected

	return nil
}
