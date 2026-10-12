package imp

type Logger interface {
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)
	Desc(args ...any)
	User(args ...any)
	Assistant(args ...any)
}
