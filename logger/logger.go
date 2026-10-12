package logger

import (
	imp "github.com/whxcode/mini-gnet/interface"
	loggerColor "github.com/whxcode/mini-gnet/logger/color"
)

var logger imp.Logger = &loggerColor.ColorLogger{}

func Info(args ...any) {
	logger.Info(args...)
}

func User(args ...any) {
	logger.User(args...)
}

func Warn(args ...any) {
	logger.Warn(args...)
}

func Assistant(args ...any) {
	logger.Assistant(args...)
}

func Error(args ...any) {
	logger.Error(args...)
}

func Desc(args ...any) {
	logger.Desc(args...)
}
