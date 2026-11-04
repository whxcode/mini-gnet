// Package query 定义用户输入的片段，以及文本引用语法，不依赖终端或任务队列。
package query

import (
	"fmt"
	"strings"
	"unicode"
)

// Kind 区分普通文字和技能引用；后续引用类型仍使用相同的 Value 字段。
type Kind string

const (
	Text  Kind = "text"
	Skill Kind = "skill"
)

// Segment 是输入的一段内容，技能名称不包含显示用的图标或颜色。
type Segment struct {
	Kind  Kind   `json:"kind"`
	Value string `json:"value"`
}

// Input 是一次提交的有序内容；进入任务时复制，避免后续编辑改变已排队的任务。
type Input []Segment

// Source 返回可复制的文本语法，供管道、命令识别和错误提示使用。
func (input Input) Source() string {
	var text strings.Builder
	for _, part := range input {
		if part.Kind == Skill {
			text.WriteString("/skills{" + part.Value + "}")
		} else {
			text.WriteString(part.Value)
		}
	}

	return text.String()
}

// Label 返回不含正文的简短提问，适合会话标题、排队提示和聊天显示。
func (input Input) Label() string {
	var text strings.Builder
	for _, part := range input {
		if part.Kind == Skill {
			text.WriteString("[技能 " + part.Value + "]")
		} else {
			text.WriteString(part.Value)
		}
	}

	return text.String()
}

// Validate 只检查已经结构化的片段，不再解析语法或读取文件。
func (input Input) Validate() error {
	if strings.TrimSpace(input.Source()) == "" {
		return fmt.Errorf("问题不能为空")
	}

	for _, part := range input {
		switch part.Kind {
		case Text:
		case Skill:
			if strings.TrimSpace(part.Value) == "" || strings.ContainsAny(part.Value, "{}\r\n") {
				return fmt.Errorf("技能引用名称不能为空，也不能包含花括号或换行")
			}
		default:
			return fmt.Errorf("暂不支持引用类型 %q", part.Kind)
		}
	}

	return nil
}

// Parse 识别明确的 /skills{名称}，代码围栏、行内代码、URL 和路径中的示例保持普通文字。
// 它只做语法解析，不查找技能，也不在这里执行任何任务。
func Parse(source string) (Input, error) {
	var parts Input
	start := 0
	codeMarker := byte(0)
	codeLength := 0

	for index := 0; index < len(source); {
		char := source[index]
		if char == '`' || char == '~' {
			end := index
			for end < len(source) && source[end] == char {
				end++
			}
			length := end - index
			if codeMarker == 0 && (char == '`' || length >= 3) {
				codeMarker = char
				codeLength = length
			} else if char == codeMarker && length == codeLength {
				codeMarker = 0
			}
			index = end
			continue
		}

		if codeMarker != 0 || !strings.HasPrefix(source[index:], "/skills{") || !Boundary(source[:index]) {
			index++
			continue
		}

		end := strings.IndexByte(source[index+8:], '}')
		if end < 0 {
			return nil, fmt.Errorf("技能引用缺少结束的 }，请使用 /skills{名称}")
		}
		end += index + 8
		name := strings.TrimSpace(source[index+8 : end])
		if name == "" || strings.ContainsAny(name, "{\r\n") {
			return nil, fmt.Errorf("技能引用名称无效，请使用 /skills{名称}")
		}

		if index > start {
			parts = append(parts, Segment{Kind: Text, Value: source[start:index]})
		}
		parts = append(parts, Segment{Kind: Skill, Value: name})
		index = end + 1
		start = index
	}

	if start < len(source) {
		parts = append(parts, Segment{Kind: Text, Value: source[start:]})
	}

	return parts, nil
}

// Boundary 只在句首、空白或中文/标点后允许引用，避免把路径及 URL 的斜线解释为命令。
func Boundary(prefix string) bool {
	if prefix == "" {
		return true
	}
	chars := []rune(prefix)
	last := chars[len(chars)-1]
	return unicode.IsSpace(last) || unicode.Is(unicode.Han, last) || strings.ContainsRune("，。；：（、", last)
}
