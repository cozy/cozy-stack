package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoggerParseLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Level
		err      error
	}{
		{
			name:     "Valid",
			input:    "error",
			expected: ErrorLevel,
			err:      nil,
		},
		{
			name:     "ValidWithShortcut",
			input:    "warn",
			expected: WarnLevel,
			err:      nil,
		},
		{
			name:     "UpperCase",
			input:    "INFO",
			expected: InfoLevel,
			err:      nil,
		},
		{
			name:     "MixUpperLowerCase",
			input:    "Debug",
			expected: DebugLevel,
			err:      nil,
		},
		{
			name:     "WithPrefix",
			input:    "-Debug",
			expected: levelUnknown,
			err:      ErrInvalidLevel,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := ParseLevel(test.input)

			assert.Equal(t, test.expected, res)
			assert.ErrorIs(t, err, test.err)
		})
	}
}

func TestLoggerLevelString(t *testing.T) {
	t.Run("ValidInput", func(t *testing.T) {
		assert.Equal(t, "error", ErrorLevel.String())
		assert.Equal(t, "debug", DebugLevel.String())
		assert.Equal(t, "warning", WarnLevel.String())
		assert.Equal(t, "info", InfoLevel.String())
	})

	t.Run("InvalidInput", func(t *testing.T) {
		assert.Equal(t, "unknown", Level(42).String())
	})
}

func TestLoggerUnmarshalText(t *testing.T) {
	t.Run("ValidInput", func(t *testing.T) {
		var lvl Level

		err := lvl.UnmarshalText([]byte("error"))
		assert.NoError(t, err)
		assert.Equal(t, ErrorLevel, lvl)
	})

	t.Run("InvalidInput", func(t *testing.T) {
		var lvl Level

		err := lvl.UnmarshalText([]byte("invalid-stuff"))
		assert.EqualError(t, err, `"invalid-stuff": not a valid logging Level`)
		assert.ErrorIs(t, err, ErrInvalidLevel)
	})
}
