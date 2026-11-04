package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestArraySchema 验证搜索结果列表逐项校验，而不是只验证 results 是一个数组。
func TestArraySchema(t *testing.T) {
	definition := json.RawMessage(`{
		"type":"object",
		"properties":{
			"results":{
				"type":"array",
				"items":{
					"type":"object",
					"properties":{"title":{"type":"string"}},
					"required":["title"],
					"additionalProperties":false
				}
			}
		},
		"required":["results"],
		"additionalProperties":false
	}`)
	parsed, err := parseSchema(definition)
	if err != nil {
		t.Fatal(err)
	}

	for _, valid := range []string{
		`{"results":[]}`,
		`{"results":[{"title":"中文"},{"title":"Go"}]}`,
	} {
		if err := parsed.validateJSON([]byte(valid)); err != nil {
			t.Fatalf("有效数组被拒绝：%v", err)
		}
	}

	for _, invalid := range []string{
		`{"results":null}`,
		`{"results":{}}`,
		`{"results":[{}]}`,
		`{"results":[{"title":42}]}`,
		`{"results":[{"title":"Go","extra":"unknown"}]}`,
	} {
		if err := parsed.validateJSON([]byte(invalid)); err == nil {
			t.Fatalf("无效数组被接受：%s", invalid)
		}
	}

	err = parsed.validateJSON([]byte(`{"results":[{"title":"ok"},{"title":42}]}`))
	if err == nil || !strings.Contains(err.Error(), "$.results[1].title") {
		t.Fatalf("错误应定位到具体数组项：%v", err)
	}

	for _, invalid := range []string{
		`{"type":"object","properties":{"results":{"type":"array"}}}`,
		`{"type":"object","items":{"type":"string"}}`,
		`{"type":"object","properties":{"results":{"type":"array","items":{"type":"unknown"}}}}`,
	} {
		if _, err := parseSchema([]byte(invalid)); err == nil {
			t.Fatalf("不完整或无效的 items 定义被接受：%s", invalid)
		}
	}
}
