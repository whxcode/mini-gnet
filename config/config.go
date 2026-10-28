package config

type Config struct {
	BaseURL    string `json:"baseUrl"`
	APIKey     string `json:"apiKey"`
	Model      string `json:"model"`
	ChatsDir   string `json:"chatsDir"`
	ProjectDir string `json:"projectDir"`
}

var DefautConfig Config = Config{
	BaseURL:  "https://api.deepseek.com",
	APIKey:   "sk-94e0d68648e84cdfa748c11d58bc42b1",
	Model:    "deepseek-reasoner",
	ChatsDir: "/Users/mac/study/mini-gnet/chat_history",
}

func LoadConfig() *Config {
	/*
		homeDir, _ := os.UserConfigDir()
		configfile := filepath.Join(homeDir, "agent-check", "config.json")

		fd, err := os.Open(configfile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				SetConfigFile()

				fd, _ = os.Open(configfile)
			} else {
				panic(err)
			}
		}

		byte, err := io.ReadAll(fd)
		if err != nil {
			panic(err)
		}

		json.Unmarshal(byte, &DefautConfig)

		// 加载配置文件.
		return &DefautConfig
	*/

	return &DefautConfig
}

func SetConfigFile() {
	/*
		// 从头开始设置配置文件.
		homeDir, _ := os.UserConfigDir()
		configDir := filepath.Join(homeDir, "agent-check")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			panic(err)
		}

		err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(""), 0o644)
		if err != nil {
			panic(err)
		}

		fmt.Println("请开始配置文件", homeDir)
	*/
}
