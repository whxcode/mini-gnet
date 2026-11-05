package input

import (
	"strings"

	"github.com/whxcode/mini-gnet/internal/query"
)

// CompletionPrefix 返回光标前正在输入的斜线词；路径、URL、代码和已有标签不会触发候选。
func (m Composer) CompletionPrefix() (string, bool) {
	buffer := []rune(m.text.Value())
	end := m.offset()
	for start := end - 1; start >= 0; start-- {
		char := buffer[start]
		if _, reference := m.references[char]; reference {
			return "", false
		}
		if char == '/' {
			prefix := m.decode(string(buffer[:start])).Source()
			if !query.Boundary(prefix) {
				return "", false
			}
			// 通过同一个解析器检查当前是否位于 Markdown 代码中。
			parts, err := query.Parse(prefix + "/skills{completion-check}")
			if err != nil || len(parts) == 0 || parts[len(parts)-1].Kind != query.Skill {
				return "", false
			}
			return string(buffer[start:end]), true
		}
		if char == ' ' || char == '\n' || char == '\t' || char == '}' {
			return "", false
		}
	}

	return "", false
}

// Complete 用一个技能标签或命令替换当前斜线词，保留光标后已有文字。
func (m *Composer) Complete(name string, skill bool) {
	prefix, ok := m.CompletionPrefix()
	if !ok {
		return
	}
	end := m.offset()
	m.moveTo(end-len([]rune(prefix)), true)
	m.text.DeleteSelection()
	if skill {
		m.InsertSkill(name)
	} else {
		m.insertText(name)
	}
}

// SkillFilter 将 /、/sk、/skills{名称} 的输入转换为技能名称过滤条件。
func SkillFilter(prefix string) string {
	if strings.HasPrefix("/skills", prefix) {
		return ""
	}

	return strings.TrimPrefix(strings.TrimPrefix(prefix, "/skills{"), "/")
}

// CompleteMCP 用 MCP 标签替换光标前的候选词，保留后面的文字。
func (m *Composer) CompleteMCP(name string) {
	prefix, ok := m.CompletionPrefix()
	if !ok {
		return
	}

	end := m.offset()
	m.moveTo(end-len([]rune(prefix)), true)
	m.text.DeleteSelection()
	m.InsertMCP(name)
}

// MCPFilter 将斜线候选词转换为服务名称过滤条件。
func MCPFilter(prefix string) string {
	if strings.HasPrefix("/mcp", prefix) {
		return ""
	}
	return strings.TrimPrefix(strings.TrimPrefix(prefix, "/mcp{"), "/")
}
