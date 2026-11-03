package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"unicode/utf8"
)

// schema 实现当前内置工具需要的 JSON Schema 子集，未知关键字加载时报错，不静默忽略。
type schema struct {
	Type                 string             `json:"type"`
	Description          string             `json:"description,omitempty"`
	Properties           map[string]*schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties bool               `json:"additionalProperties"`
	MinLength            int                `json:"minLength,omitempty"`
}

var toolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// decodeJSON 要求完整的一个 JSON 值，避免尾随内容或未知定义字段被忽略。
func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("必须提供一个完整 JSON 值，不能包含尾随内容")
	}

	return nil
}

// ParseDefinition 读取嵌入的定义文件，并在启动时发现名称或 schema 错误。
func ParseDefinition(data []byte) (Definition, error) {
	var definition Definition

	if err := decodeJSON(data, &definition); err != nil {
		return definition, err
	}

	if definition.SchemaVersion != 1 ||
		!toolName.MatchString(definition.Name) ||
		definition.Description == "" {
		return definition, fmt.Errorf("工具定义需要 schema_version=1、有效名称和说明")
	}

	if _, err := parseSchema(definition.InputSchema); err != nil {
		return definition, fmt.Errorf("%s input_schema: %w", definition.Name, err)
	}

	if _, err := parseSchema(definition.OutputSchema); err != nil {
		return definition, fmt.Errorf("%s output_schema: %w", definition.Name, err)
	}

	return definition, nil
}

// parseSchema 解析并检查本版支持的约束，根节点必须是 object。
func parseSchema(data []byte) (*schema, error) {
	var s schema

	if err := decodeJSON(data, &s); err != nil {
		return nil, err
	}

	if s.Type != "object" {
		return nil, fmt.Errorf("schema 根节点必须为 object")
	}

	return &s, s.check()
}

// check 检查 schema 本身，当前支持 object、string 和 integer。
func (s *schema) check() error {
	if s == nil {
		return fmt.Errorf("属性 schema 不能为空")
	}

	switch s.Type {
	case "object":
		if s.MinLength != 0 {
			return fmt.Errorf("object 不支持 minLength")
		}

		for _, name := range s.Required {
			if _, ok := s.Properties[name]; !ok {
				return fmt.Errorf("required 属性 %s 未定义", name)
			}
		}

		for name, property := range s.Properties {
			if err := property.check(); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}

	case "string", "integer":
		if len(s.Properties) != 0 || len(s.Required) != 0 || s.AdditionalProperties {
			return fmt.Errorf("%s 不支持对象约束", s.Type)
		}

		if s.MinLength < 0 || s.Type == "integer" && s.MinLength != 0 {
			return fmt.Errorf("无效的 minLength")
		}

	default:
		return fmt.Errorf("暂不支持 schema 类型 %q", s.Type)
	}

	return nil
}

// validateJSON 校验输入或输出数据，不把 JSON null 当作空对象。
func (s *schema) validateJSON(data []byte) error {
	var value any

	if err := decodeJSON(data, &value); err != nil {
		return err
	}

	return s.validate(value, "$")
}

// validate 递归校验必填、额外属性、类型和字符串最小长度。
func (s *schema) validate(value any, path string) error {
	switch s.Type {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s 必须是 object", path)
		}

		for _, name := range s.Required {
			if _, ok := object[name]; !ok {
				return fmt.Errorf("%s 缺少必填参数 %s", path, name)
			}
		}

		for name, field := range object {
			property, exists := s.Properties[name]
			if !exists {
				if !s.AdditionalProperties {
					return fmt.Errorf("%s 不接受参数 %s", path, name)
				}

				continue
			}

			if err := property.validate(field, path+"."+name); err != nil {
				return err
			}
		}

	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s 必须是 string", path)
		}

		if utf8.RuneCountInString(text) < s.MinLength {
			return fmt.Errorf("%s 长度不能小于 %d", path, s.MinLength)
		}

	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("%s 必须是 integer", path)
		}

		if _, err := number.Int64(); err != nil {
			return fmt.Errorf("%s 必须是整数", path)
		}
	}

	return nil
}
