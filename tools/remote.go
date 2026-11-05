package tools

import (
	"encoding/json"
	"fmt"
)

// WithRemote 创建本次任务的注册表副本，保留远端完整 schema，由远端校验业务约束。
// 共享注册表始终只读；本地仍校验工具名、JSON 对象和名称冲突。
func (r *Registry) WithRemote(implementations ...Tool) (*Registry, error) {
	next := &Registry{entries: make(map[string]entry)}
	if r != nil {
		for name, item := range r.entries {
			next.entries[name] = item
		}

		next.order = append([]string(nil), r.order...)
	}

	for _, implementation := range implementations {
		if implementation == nil {
			return nil, fmt.Errorf("远程工具执行入口不能为空")
		}

		definition := implementation.Definition().Clone()
		if !toolName.MatchString(definition.Name) {
			return nil, fmt.Errorf("远程工具名称无效：%s", definition.Name)
		}

		if _, exists := next.entries[definition.Name]; exists {
			return nil, fmt.Errorf("工具名称重复：%s", definition.Name)
		}

		var input map[string]json.RawMessage
		if err := json.Unmarshal(definition.InputSchema, &input); err != nil || input == nil {
			return nil, fmt.Errorf("远程工具 %s 的 inputSchema 必须是 JSON 对象", definition.Name)
		}

		var rootType string
		if err := json.Unmarshal(input["type"], &rootType); err != nil || rootType != "object" {
			return nil, fmt.Errorf("远程工具 %s 的 inputSchema 根类型必须为 object", definition.Name)
		}

		next.entries[definition.Name] = entry{
			definition: definition,
			tool:       implementation,
			input:      &schema{Type: "object", AdditionalProperties: true},
			output:     &schema{Type: "object", AdditionalProperties: true},
		}

		next.order = append(next.order, definition.Name)
	}

	return next, nil
}
