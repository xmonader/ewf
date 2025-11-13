package ewf

type Logger interface {
	Info() LoggerEvent
	Error() LoggerEvent
	Warn() LoggerEvent
	Debug() LoggerEvent
}

type LoggerEvent interface {
	Err(error) LoggerEvent
	Str(string, string) LoggerEvent
	Int(string, int) LoggerEvent
	Msg(string)
}
