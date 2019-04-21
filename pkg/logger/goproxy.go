package logger

type NilLogger struct{}

func (NilLogger) Printf(format string, v ...interface{}) {}
