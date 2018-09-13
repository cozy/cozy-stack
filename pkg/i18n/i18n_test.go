package i18n

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTranslate(t *testing.T) {
	LoadLocale("fr", []byte(`
msgid "english"
msgstr "french"

msgid "hello %s"
msgstr "bonjour %s"
`))

	s := Translate("english", "fr")
	assert.Equal(t, "french", s)
	s = Translate("hello %s", "fr", "toto")
	assert.Equal(t, "bonjour toto", s)

	s = Translate("english", "en")
	assert.Equal(t, "english", s)
	s = Translate("hello %s", "en", "toto")
	assert.Equal(t, "hello toto", s)
}
