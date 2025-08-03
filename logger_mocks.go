package ewf

// MockLogger for testing
type MockLogger struct {
	InfoCalled  bool
	ErrorCalled bool
	WarnCalled  bool
	DebugCalled bool
}

func (m *MockLogger) Info() LoggerEvent  { return &MockLoggerEvent{logger: m, level: "info"} }
func (m *MockLogger) Error() LoggerEvent { return &MockLoggerEvent{logger: m, level: "error"} }
func (m *MockLogger) Warn() LoggerEvent  { return &MockLoggerEvent{logger: m, level: "warn"} }
func (m *MockLogger) Debug() LoggerEvent { return &MockLoggerEvent{logger: m, level: "debug"} }

type MockLoggerEvent struct {
	logger *MockLogger
	level  string
}

func (m *MockLoggerEvent) Err(error) LoggerEvent          { return m }
func (m *MockLoggerEvent) Str(string, string) LoggerEvent { return m }
func (m *MockLoggerEvent) Int(string, int) LoggerEvent    { return m }
func (m *MockLoggerEvent) Msg(string) {
	switch m.level {
	case "info":
		m.logger.InfoCalled = true
	case "error":
		m.logger.ErrorCalled = true
	case "warn":
		m.logger.WarnCalled = true
	case "debug":
		m.logger.DebugCalled = true
	}
}
