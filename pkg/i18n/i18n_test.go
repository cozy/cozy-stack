package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTranslate(t *testing.T) {
	contextName := "foo"

	LoadLocale("fr", "", []byte(`
msgid "english"
msgstr "french"

msgid "hello %s"
msgstr "bonjour %s"

msgid "context"
msgstr "contexte"
`))

	LoadLocale("fr", contextName, []byte(`
msgid "english"
msgstr "french"

msgid "hello %s"
msgstr "bonjour %s"

msgid "context"
msgstr "contexte foo"
`))

	s := Translate("english", "fr", contextName)
	assert.Equal(t, "french", s)
	s = Translate("hello %s", "fr", contextName, "toto")
	assert.Equal(t, "bonjour toto", s)

	s = Translate("english", "en", contextName)
	assert.Equal(t, "english", s)
	s = Translate("hello %s", "en", contextName, "toto")
	assert.Equal(t, "hello toto", s)

	s = Translate("context", "fr", contextName)
	assert.Equal(t, "contexte foo", s)
	s = Translate("context", "fr", "bar")
	assert.Equal(t, "contexte", s)
}
