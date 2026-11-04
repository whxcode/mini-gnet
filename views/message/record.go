package message

import (
	"encoding/json"

	"github.com/whxcode/mini-gnet/internal/query"
	"github.com/whxcode/mini-gnet/tools"
)

// MarshalJSON 保留 SDK 对消息正文的编码，再附加本地标记。
// 必须显式实现：嵌入的 SDK 消息自带 MarshalJSON，否则会忽略本地元数据字段。
func (r Record) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(r.ChatCompletionMessage)
	if err != nil || (r.Response == nil && r.Execution == nil && len(r.Input) == 0) {
		return data, err
	}

	var fields map[string]json.RawMessage

	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}

	if r.Response != nil {
		metadata, err := json.Marshal(r.Response)
		if err != nil {
			return nil, err
		}

		fields["agent_response"] = metadata
	}

	if r.Execution != nil {
		metadata, err := json.Marshal(r.Execution)
		if err != nil {
			return nil, err
		}

		fields["tool_execution"] = metadata
	}

	if len(r.Input) > 0 {
		metadata, err := json.Marshal(r.Input)
		if err != nil {
			return nil, err
		}

		fields["user_input"] = metadata
	}

	return json.Marshal(fields)
}

// UnmarshalJSON 分别读取 SDK 消息和本地标记，旧文件没有标记时仍能正常加载。
func (r *Record) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &r.ChatCompletionMessage); err != nil {
		return err
	}

	var metadata struct {
		Input     query.Input          `json:"user_input"`
		Response  *ResponseInfo        `json:"agent_response"`
		Execution *tools.ExecutionInfo `json:"tool_execution"`
	}

	if err := json.Unmarshal(data, &metadata); err != nil {
		return err
	}

	r.Response = metadata.Response
	r.Execution = metadata.Execution
	r.Input = metadata.Input

	return nil
}
