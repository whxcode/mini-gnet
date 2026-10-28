package config

type Config struct {
	BaseURL    string `json:"baseUrl"`
	APIKey     string `json:"apiKey"`
	Model      string `json:"model"`
	ChatsDir   string `json:"chatsDir"`
	ProjectDir string `json:"projectDir"`
}

/*
  "baseUrl": "http://localhost:11434/v1",
  "apiKey": "",
  "model": "qwen2.5:1.5b",
  "chatsDir": "/Users/mac/study/mini-gnet/chat_history"
}
*/

var DefautConfig Config = Config{
	BaseURL:  "http://localhost:11434/v1",
	APIKey:   "",
	Model:    "qwen3:8b",
	ChatsDir: "/Users/mac/study/mini-gnet/chat_history",
}

// LoadConfig 保留原调用方式；新代码使用 Load 获取明确的错误信息。
func LoadConfig() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return &cfg
}
