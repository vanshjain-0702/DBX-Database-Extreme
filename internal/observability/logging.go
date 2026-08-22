package observability

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger wraps zerolog for structured logging.
type Logger struct {
	log zerolog.Logger
}

// NewLogger creates a logger with the given level and format.
func NewLogger(level, format string) *Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	var log zerolog.Logger
	if format == "console" {
		log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
			Level(lvl).
			With().Timestamp().Logger()
	} else {
		log = zerolog.New(os.Stderr).Level(lvl).With().Timestamp().Logger()
	}
	return &Logger{log: log}
}

func (l *Logger) Info(msg string, fields ...interface{})  { l.log.Info().Msg(fmt.Sprintf(msg, fields...)) }
func (l *Logger) Error(msg string, fields ...interface{}) { l.log.Error().Msg(fmt.Sprintf(msg, fields...)) }
func (l *Logger) Warn(msg string, fields ...interface{})  { l.log.Warn().Msg(fmt.Sprintf(msg, fields...)) }
func (l *Logger) Debug(msg string, fields ...interface{}) { l.log.Debug().Msg(fmt.Sprintf(msg, fields...)) }

// TraceSpan represents a tracing span (simplified).
type TraceSpan struct {
	Name      string
	StartTime time.Time
	Tags      map[string]string
}

// StartSpan starts a new trace span.
func StartSpan(ctx context.Context, name string) (*TraceSpan, context.Context) {
	span := &TraceSpan{
		Name:      name,
		StartTime: time.Now(),
		Tags:      make(map[string]string),
	}
	return span, ctx
}

// Finish ends the span.
func (s *TraceSpan) Finish() time.Duration {
	return time.Since(s.StartTime)
}

// SetTag adds a tag to the span.
func (s *TraceSpan) SetTag(key, value string) {
	s.Tags[key] = value
}
