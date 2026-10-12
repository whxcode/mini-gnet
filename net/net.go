package net

import (
	"context"
	"errors"
	"io"
	"net/http"
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

func (c *Client) Stream(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool, onText func(string) error) (StreamResult, error) {
	reqs := openai.ChatCompletionRequest{
		Model:    c.Model,
		Messages: messages,
		Stream:   true,
		Tools:    tools,
	}

	// Ollama 的 OpenAI 兼容接口
	openaiConfig := openai.DefaultConfig(c.ApiKey)
	openaiConfig.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.HTTP != nil {
		openaiConfig.HTTPClient = c.HTTP
	}
	client := openai.NewClientWithConfig(openaiConfig)
	stream, err := client.CreateChatCompletionStream(ctx, reqs)
	if err != nil {
		return StreamResult{}, err
	}
	defer stream.Close()
	result := StreamResult{}
	toolCalls := make(map[int]*openai.ToolCall)

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return StreamResult{}, err
		}
		if len(response.Choices) == 0 {
			continue
		}
		choice := response.Choices[0]
		result.Finish = choice.FinishReason
		if choice.Delta.Content != "" {
			result.Text += choice.Delta.Content
			if onText != nil {
				if err := onText(choice.Delta.Content); err != nil {
					return StreamResult{}, err
				}
			}
		}
		for _, delta := range choice.Delta.ToolCalls {
			index := 0
			if delta.Index != nil {
				index = *delta.Index
			}
			call := toolCalls[index]
			if call == nil {
				copy := delta
				call = &copy
				toolCalls[index] = call
			} else {
				call.ID += delta.ID
				call.Function.Name += delta.Function.Name
				call.Function.Arguments += delta.Function.Arguments
			}
		}
	}
	for i := 0; i < len(toolCalls); i++ {
		if call, ok := toolCalls[i]; ok {
			call.Index = nil
			result.ToolCalls = append(result.ToolCalls, *call)
		}
	}
	return result, nil
}
