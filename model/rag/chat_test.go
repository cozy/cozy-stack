package rag

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForeachSSE(t *testing.T) {
	t.Run("normal events are passed to callback", func(t *testing.T) {
		input := `data: {"object":"chat.completion.chunk","content":"hello"}

data: {"object":"chat.completion.chunk","content":"world"}

data: [DONE]
`
		var events []map[string]interface{}
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {
			events = append(events, event)
		})

		require.NoError(t, err)
		assert.Len(t, events, 2)
		assert.Equal(t, "hello", events[0]["content"])
		assert.Equal(t, "world", events[1]["content"])
	})

	t.Run("error event with code and message returns formatted error", func(t *testing.T) {
		input := `data: {"error":{"message":"Error while generating answer","code":"ERROR_ANSWER_GENERATION"}}
`
		var events []map[string]interface{}
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {
			events = append(events, event)
		})

		require.Error(t, err)
		assert.Equal(t, "ERROR_ANSWER_GENERATION: Error while generating answer", err.Error())
		assert.Empty(t, events)
	})

	t.Run("error event with only message returns error", func(t *testing.T) {
		input := `data: {"error":{"message":"Something went wrong"}}
`
		var events []map[string]interface{}
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {
			events = append(events, event)
		})

		require.Error(t, err)
		assert.Equal(t, "Something went wrong", err.Error())
		assert.Empty(t, events)
	})

	t.Run("error event with empty message returns unknown error", func(t *testing.T) {
		input := `data: {"error":{"message":"","code":"SOME_CODE"}}
`
		var events []map[string]interface{}
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {
			events = append(events, event)
		})

		require.Error(t, err)
		assert.Equal(t, "SOME_CODE: unknown streaming error", err.Error())
		assert.Empty(t, events)
	})

	t.Run("error event with no message field returns unknown error", func(t *testing.T) {
		input := `data: {"error":{}}
`
		var events []map[string]interface{}
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {
			events = append(events, event)
		})

		require.Error(t, err)
		assert.Equal(t, "unknown streaming error", err.Error())
		assert.Empty(t, events)
	})

	t.Run("DONE stops processing", func(t *testing.T) {
		input := `data: {"object":"first"}

data: [DONE]
data: {"object":"should not be processed"}
`
		var events []map[string]interface{}
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {
			events = append(events, event)
		})

		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "first", events[0]["object"])
	})

	t.Run("invalid SSE format returns error", func(t *testing.T) {
		input := `invalid line without colon
`
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {})

		require.Error(t, err)
		assert.Equal(t, "invalid SSE response", err.Error())
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		input := `data: {invalid json}
`
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid character")
	})

	t.Run("comments are skipped", func(t *testing.T) {
		input := `: this is a comment
data: {"object":"event"}

data: [DONE]
`
		var events []map[string]interface{}
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {
			events = append(events, event)
		})

		require.NoError(t, err)
		assert.Len(t, events, 1)
	})

	t.Run("empty lines are skipped", func(t *testing.T) {
		input := `

data: {"object":"event"}


data: [DONE]
`
		var events []map[string]interface{}
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {
			events = append(events, event)
		})

		require.NoError(t, err)
		assert.Len(t, events, 1)
	})

	t.Run("non-data fields are skipped", func(t *testing.T) {
		input := `event: message
id: 123
data: {"object":"event"}

data: [DONE]
`
		var events []map[string]interface{}
		err := foreachSSE(strings.NewReader(input), func(event map[string]interface{}) {
			events = append(events, event)
		})

		require.NoError(t, err)
		assert.Len(t, events, 1)
	})
}
