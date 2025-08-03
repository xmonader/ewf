package ewf

import "github.com/rs/zerolog"

// ZerologAdapter adapts zerolog.Logger to implement the ewf.Logger interface
type ZerologAdapter struct {
	logger zerolog.Logger
}

func NewZerologAdapter(logger zerolog.Logger) *ZerologAdapter {
	return &ZerologAdapter{logger: logger}
}

func (z *ZerologAdapter) Info() LoggerEvent {
	return &zerologEvent{event: z.logger.Info()}
}

func (z *ZerologAdapter) Error() LoggerEvent {
	return &zerologEvent{event: z.logger.Error()}
}

func (z *ZerologAdapter) Warn() LoggerEvent {
	return &zerologEvent{event: z.logger.Warn()}
}

func (z *ZerologAdapter) Debug() LoggerEvent {
	return &zerologEvent{event: z.logger.Debug()}
}

type zerologEvent struct {
	event *zerolog.Event
}

func (z *zerologEvent) Err(err error) LoggerEvent {
	return &zerologEvent{event: z.event.Err(err)}
}

func (z *zerologEvent) Str(key, val string) LoggerEvent {
	return &zerologEvent{event: z.event.Str(key, val)}
}

func (z *zerologEvent) Int(key string, val int) LoggerEvent {
	return &zerologEvent{event: z.event.Int(key, val)}
}

func (z *zerologEvent) Msg(msg string) {
	z.event.Msg(msg)
}
