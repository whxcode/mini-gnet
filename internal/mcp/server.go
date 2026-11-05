// Package mcp 管理服务配置，并提供 MCP 协议连接、工具发现和远程调用。
package mcp

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const StreamableHTTP = "streamable_http"

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Server 描述如何找到一个服务，不包含连接状态或远程工具列表。
type Server struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Transport   string `json:"transport"`
	URL         string `json:"url"`
}

// Normalize 校验本地字段格式，不访问网络，也不承诺地址提供了 MCP 服务。
func Normalize(server Server) (Server, error) {
	server.Name = strings.TrimSpace(server.Name)
	server.Description = strings.TrimSpace(server.Description)
	server.URL = strings.TrimSpace(server.URL)

	if !validName.MatchString(server.Name) {
		return Server{}, fmt.Errorf("服务名称须为 1–64 位字母、数字、下划线或短横线，且以字母或数字开头")
	}

	if server.Transport != StreamableHTTP {
		return Server{}, fmt.Errorf("服务 %s：当前只支持 transport=%s", server.Name, StreamableHTTP)
	}

	address, err := url.Parse(server.URL)
	if err != nil || address.Hostname() == "" ||
		(address.Scheme != "http" && address.Scheme != "https") ||
		address.User != nil || address.Fragment != "" {
		return Server{}, fmt.Errorf("服务 %s：地址须为 http(s) URL，不包含账号或片段", server.Name)
	}

	return server, nil
}
