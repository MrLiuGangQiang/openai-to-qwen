// Package logger provides a minimal leveled logger. When the configured level
// is below a message's level, the message is skipped before any formatting so
// the hot path pays no cost at all (LOG_LEVEL defaults to off).
package logger

import (
	"io"
	"log"
	"strings"
)

// Level controls how much is logged.
type Level int

const (
	LevelOff Level = iota
	LevelError
	LevelInfo
	LevelDebug
)

// Parse converts a LOG_LEVEL string to a Level. Unknown/empty -> Off.
func Parse(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "error":
		return LevelError
	default:
		return LevelOff
	}
}

// Logger is a leveled logger writing to one underlying writer.
type Logger struct {
	level Level
	w     io.Writer
	elog  *log.Logger
	ilog  *log.Logger
	dlog  *log.Logger
}

// New creates a Logger. w is typically os.Stdout (captured by Docker).
func New(level Level, w io.Writer) *Logger {
	return &Logger{
		level: level,
		w:     w,
		elog:  log.New(w, "ERROR ", log.LstdFlags),
		ilog:  log.New(w, "INFO  ", log.LstdFlags),
		dlog:  log.New(w, "DEBUG ", log.LstdFlags),
	}
}

// Enabled reports whether messages at level will be emitted.
func (l *Logger) Enabled(level Level) bool { return l != nil && l.level >= level }

// Errorf logs at error level.
func (l *Logger) Errorf(format string, args ...any) {
	if l.Enabled(LevelError) {
		l.elog.Printf(format, args...)
	}
}

// Infof logs at info level.
func (l *Logger) Infof(format string, args ...any) {
	if l.Enabled(LevelInfo) {
		l.ilog.Printf(format, args...)
	}
}

// Debugf logs at debug level.
func (l *Logger) Debugf(format string, args ...any) {
	if l.Enabled(LevelDebug) {
		l.dlog.Printf(format, args...)
	}
}

// gatedWriter drops writes below the given level (used for *log.Logger
// consumers such as http.ReverseProxy.ErrorLog).
type gatedWriter struct {
	l     *Logger
	level Level
}

func (w *gatedWriter) Write(p []byte) (int, error) {
	if w.l.level >= w.level {
		return w.l.w.Write(p)
	}
	return len(p), nil
}

// StdLogger returns a *log.Logger whose output is gated at level.
func (l *Logger) StdLogger(level Level) *log.Logger {
	return log.New(&gatedWriter{l: l, level: level}, "PROXY ", log.LstdFlags)
}
