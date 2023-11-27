package logger

import (
	"errors"
	"fmt"
	"strings"
)

// Level type
type Level uint8

// These are the different logging levels.
const (
	// levelUnknown represent an unparsable level
	levelUnknown Level = iota

	// ErrorLevel logs important errors when failure append.
	ErrorLevel

	// WarnLevel logs non critical entries that deserve some intention.
	WarnLevel

	// InfoLevel log general operational entries about what's going on inside
	// the application.
	InfoLevel

	// DebugLevel logs is only enabled when debugging is setup. It can be
	// very verbose logging and should be activated only on a limited period.
	DebugLevel
)

var (
	ErrInvalidLevel = errors.New("not a valid logging Level")
)

// String converts the Level to a string. E.g. LevelDebug becomes "debug".
func (level Level) String() string {
	if b, err := level.MarshalText(); err == nil {
		return string(b)
	}

	return "unknown"
}

// ParseLevel takes a string level and returns the log level constant.
func ParseLevel(lvl string) (Level, error) {
	switch strings.ToLower(lvl) {
	case "error":
		return ErrorLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "info":
		return InfoLevel, nil
	case "debug":
		return DebugLevel, nil
	}

	return levelUnknown, fmt.Errorf("%q: %w", lvl, ErrInvalidLevel)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (level *Level) UnmarshalText(text []byte) error {
	l, err := ParseLevel(string(text))
	if err != nil {
		return err
	}

	*level = l

	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (level Level) MarshalText() ([]byte, error) {
	switch level {
	case DebugLevel:
		return []byte("debug"), nil
	case InfoLevel:
		return []byte("info"), nil
	case WarnLevel:
		return []byte("warning"), nil
	case ErrorLevel:
		return []byte("error"), nil
	}

	return nil, fmt.Errorf("not a valid logging level %d", level)
}
