// Package config provides configuration management for the application.
package config

type Config struct {
	BaseURL       string `json:"baseUrl"`
	APIKey        string `-`
	Model         string `json:"model"`
	ChatsDir      string `json:"chatsDir"`
	ProjectDir    string `json:"projectDir"`
	ToolsDir      string `json:"toolsDir"`                // 外部工具目录；相对路径以项目目录为起点。
	ContextWindow int    `json:"contextWindow,omitempty"` // 当前服务实际使用的上下文上限；0 表示未知，仅供占比展示。
}

/*
  "baseUrl": "http://localhost:11434/v1",
  "apiKey": "",
  "model": "qwen2.5:1.5b",
  "chatsDir": "/Users/mac/study/mini-gnet/chat_history"
}
*/

var defautConfig Config = Config{
	BaseURL:  "http://localhost:11434/v1",
	APIKey:   "",
	Model:    "qwen3:8b",
	ChatsDir: "/Users/mac/study/mini-gnet/chat_history",
	ToolsDir: "my_tools",
}

// LoadConfig 保留原调用方式；新代码使用 Load 获取明确的错误信息。
func LoadConfig() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}

	return &cfg
}
