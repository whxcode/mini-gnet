// loggerColor package provides a color logger implementation for the mini-gnet framework. It implements the Logger interface defined in the imp package, allowing for logging of informational, warning, and error messages with color formatting.
package loggerColor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	imp "github.com/whxcode/mini-gnet/interface"
)

var (
	infoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))

	userStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	assistantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))

	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94A3B8"))
)

const (
	IconRobot = "󰚩" // nf-md-robot
	IconUser  = "" // nf-fa-user
	IconError = "" // nf-fa-times-circle
	IconWarn  = "" // nf-fa-warning
	IconInfo  = "" // nf-fa-info-circle
	IconOK    = "" // nf-fa-check
)

type ColorLogger struct{}

var _ imp.Logger = (*ColorLogger)(nil)

func formatArgs(args ...any) string {
	return strings.TrimSpace(fmt.Sprintln(args...))
}

func render(style lipgloss.Style, icon string, args ...any) string {
	text := formatArgs(args...)
	if text == "" {
		return style.Render(icon)
	}

	return style.Render(icon + " " + text)
}

func (c *ColorLogger) Info(args ...any) {
	fmt.Println(render(infoStyle, IconInfo, args...))
}

func (c *ColorLogger) User(args ...any) {
	fmt.Println(render(userStyle, IconUser, args...))
}

func (c *ColorLogger) Assistant(args ...any) {
	fmt.Println(render(assistantStyle, IconRobot, args...))
}

func (c *ColorLogger) Error(args ...any) {
	fmt.Println(render(errorStyle, IconError, args...))
}

func (c *ColorLogger) Warn(args ...any) {
	fmt.Println(render(warnStyle, IconWarn, args...))
}

func (c *ColorLogger) Desc(args ...any) {
	fmt.Print(render(descStyle, "", args...))
}
