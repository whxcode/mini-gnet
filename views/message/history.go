package message

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

type SessionInfo struct {
	ID        string
	Title     string
	UpdatedAt time.Time
	Count     int
	Problem   string
}

// readHistory 使用 Reader 而不是默认 Scanner，工具输出超过 64KB 时也能恢复。
// 损坏的记录明确报错，不截断文件，也不悄悄丢掉历史后继续追加。
func readHistory(path string) ([]openai.ChatCompletionMessage, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	var messages []openai.ChatCompletionMessage
	needsNewline := false
	for line := 1; ; line++ {
		data, readErr := r.ReadBytes('\n')
		if len(data) > 0 {
			needsNewline = data[len(data)-1] != '\n'
		}
		if len(bytes.TrimSpace(data)) > 0 {
			var msg openai.ChatCompletionMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				return nil, false, fmt.Errorf("第 %d 行不是有效的聊天记录：%w", line, err)
			}
			switch msg.Role {
			case "system", "user", "assistant", "tool":
			default:
				return nil, false, fmt.Errorf("第 %d 行的角色无效", line)
			}
			messages = append(messages, msg)
		}
		if readErr == io.EOF {
			return messages, needsNewline, nil
		}
		if readErr != nil {
			return nil, false, readErr
		}
	}
}

func List(chatsDir string) ([]SessionInfo, error) {
	entries, err := os.ReadDir(chatsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sessions []SessionInfo
	for _, entry := range entries {
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		s := SessionInfo{ID: strings.TrimSuffix(entry.Name(), ".jsonl"), UpdatedAt: info.ModTime()}
		messages, _, err := readHistory(filepath.Join(chatsDir, entry.Name()))
		if err != nil {
			s.Title, s.Problem = "无法读取的会话", err.Error()
		} else {
			s.Count = len(messages)
			for _, msg := range messages {
				if msg.Role == "user" {
					s.Title = strings.Join(strings.Fields(msg.Content), " ")
					if s.Title == "" {
						s.Title = "未命名会话"
					}
					break
				}
			}
			// 启动后尚未提问的空会话不占据选择列表。
			if s.Title == "" {
				continue
			}
		}
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func Resume(chatsDir, id string) (*TempMessage, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return nil, fmt.Errorf("无效的会话 ID")
	}
	path := filepath.Join(chatsDir, id+".jsonl")
	chats, needsNewline, err := readHistory(path)
	if err != nil {
		return nil, err
	}
	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	// 有些旧文件最后一行没有换行。先补分隔符，避免下条 JSON 与它粘在一起。
	if needsNewline {
		if _, err := fd.WriteString("\n"); err != nil {
			fd.Close()
			return nil, err
		}
	}
	return &TempMessage{SessionID: id, fd: fd, chats: chats}, nil
}
