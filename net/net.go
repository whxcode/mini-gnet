// Package net 封装模型的网络协议；调用者主动读取流，不向网络层传业务回调。
package net

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/sashabaranov/go-openai"
)

type Client struct {
	BaseURL string
	Model   string
	ApiKey  string
	HTTP    *http.Client
}

type StreamResult struct {
	Text      string
	ToolCalls []openai.ToolCall
	Finish    openai.FinishReason
}

// ResponseStream 描述读取协议，测试用假流替代真实网络，不需要业务回调。
type ResponseStream interface {
	// Next 读取下一个响应片段；io.EOF 表示本次回答读取完毕。
	Next() (string, error)
	// Result 返回已累计的文字和工具调用，通常在 io.EOF 后使用。
	Result() StreamResult
	// Close 释放连接；调用者取得流后应立即 defer Close。
	Close() error
}

type responseStream struct {
	stream *openai.ChatCompletionStream
	text   strings.Builder
	calls  map[int]*openai.ToolCall
	finish openai.FinishReason
}

// OpenStream 发起模型请求并返回可读取的流；不启动协程，也不调用界面。
func (c *Client) OpenStream(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool) (ResponseStream, error) {
	cfg := openai.DefaultConfig(c.ApiKey)
	cfg.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.HTTP != nil {
		cfg.HTTPClient = c.HTTP
	}
	stream, err := openai.NewClientWithConfig(cfg).CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model: c.Model, Messages: messages, Stream: true, Tools: tools,
	})
	if err != nil {
		return nil, err
	}
	return &responseStream{stream: stream, calls: make(map[int]*openai.ToolCall)}, nil
}

// Next 读取一帧并拼接协议片段。只有工具信息的帧返回空文字，调用者继续读取即可。
func (s *responseStream) Next() (string, error) {
	response, err := s.stream.Recv()
	if err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", nil
	}
	choice := response.Choices[0]
	if choice.FinishReason != "" {
		s.finish = choice.FinishReason
	}
	s.text.WriteString(choice.Delta.Content)
	for _, delta := range choice.Delta.ToolCalls {
		index := 0
		if delta.Index != nil {
			index = *delta.Index
		}
		call := s.calls[index]
		if call == nil {
			copy := delta
			s.calls[index] = &copy
			continue
		}
		call.ID += delta.ID
		if delta.Type != "" {
			call.Type = delta.Type
		}
		call.Function.Name += delta.Function.Name
		call.Function.Arguments += delta.Function.Arguments
	}
	return choice.Delta.Content, nil
}

// Result 按工具索引排序并返回副本，避免多工具片段乱序或索引不连续造成遗漏。
func (s *responseStream) Result() StreamResult {
	result := StreamResult{Text: s.text.String(), Finish: s.finish}
	indices := make([]int, 0, len(s.calls))
	for index := range s.calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		call := *s.calls[index]
		call.Index = nil
		result.ToolCalls = append(result.ToolCalls, call)
	}
	return result
}

// Close 关闭本次 HTTP 响应，正常结束和提前取消都由调用者执行。
func (s *responseStream) Close() error { return s.stream.Close() }

var _ ResponseStream = (*responseStream)(nil)
