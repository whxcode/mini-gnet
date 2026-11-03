package message

import "encoding/json"

// MarshalJSON 保留 SDK 对消息正文的编码，再附加本地标记。
// 必须显式实现：嵌入的 ChatCompletionMessage 自带 MarshalJSON，否则会忽略 Response 字段。
func (r Record) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(r.ChatCompletionMessage)
	if err != nil || r.Response == nil {
		return data, err
	}

	var fields map[string]json.RawMessage

	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}

	metadata, err := json.Marshal(r.Response)
	if err != nil {
		return nil, err
	}

	fields["agent_response"] = metadata

	return json.Marshal(fields)
}

// UnmarshalJSON 分别读取 SDK 消息和本地标记，旧文件没有标记时仍能正常加载。
func (r *Record) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &r.ChatCompletionMessage); err != nil {
		return err
	}

	var metadata struct {
		Response *ResponseInfo `json:"agent_response"`
	}

	if err := json.Unmarshal(data, &metadata); err != nil {
		return err
	}

	r.Response = metadata.Response

	return nil
}
