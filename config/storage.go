package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Path 使用各平台约定的配置目录。独立使用 mini-gnet 命名空间，
// 避免误加载之前实验用的 agent-check 配置并覆盖当前模型设置。
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mini-gnet", "config.json"), nil
}

func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	return loadFile(path, DefautConfig)
}

func loadFile(path string, defaults Config) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Normalize(defaults)
	}
	if err != nil {
		return Config{}, err
	}
	cfg := defaults
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("配置文件 %s 无效：%w", path, err)
	}
	return Normalize(cfg)
}

func Normalize(cfg Config) (Config, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Config{}, fmt.Errorf("接口地址应为 http(s) URL，不包含账号、查询参数或片段")
	}
	if strings.HasSuffix(u.Path, "/anthropic") {
		return Config{}, fmt.Errorf("当前客户端使用 OpenAI 兼容接口，请移除地址末尾的 /anthropic")
	}
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Model == "" {
		return Config{}, fmt.Errorf("模型名称不能为空")
	}
	if strings.TrimSpace(cfg.ChatsDir) == "" {
		return Config{}, fmt.Errorf("聊天记录目录不能为空")
	}
	if cfg.ChatsDir, err = absoluteDir(cfg.ChatsDir); err != nil {
		return Config{}, err
	}
	if cfg.ProjectDir != "" {
		if cfg.ProjectDir, err = absoluteDir(cfg.ProjectDir); err != nil {
			return Config{}, err
		}
		info, err := os.Stat(cfg.ProjectDir)
		if err != nil || !info.IsDir() {
			return Config{}, fmt.Errorf("工作目录不存在或不是目录")
		}
	}
	return cfg, nil
}

func absoluteDir(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "~" || strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, strings.TrimLeft(strings.TrimPrefix(value, "~"), `/\`))
	}
	return filepath.Abs(value)
}

func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return saveFile(path, cfg)
}

func saveFile(path string, cfg Config) error {
	cfg, err := Normalize(cfg)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// 先写临时文件再替换，避免保存中断时把旧配置截断；临时文件默认权限为 0600。
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(data, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
