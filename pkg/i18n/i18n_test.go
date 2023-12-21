package i18n

import (
	"html/template"
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

func TestTranslatorHTML(t *testing.T) {
	contextName := "foo"

	LoadLocale("fr", "", []byte(`
msgid "english"
msgstr "french"

msgid "hello %s"
msgstr "bonjour **%s**"

msgid "context"
msgstr "contexte"
`))

	LoadLocale("fr", contextName, []byte(`
msgid "english"
msgstr "french"

msgid "hello %s"
msgstr "bonjour **%s**"

msgid "context"
msgstr ""
"contexte\n"
"foo"
`))

	frHTML := TranslatorHTML("fr", contextName)

	h := frHTML("english")
	assert.Equal(t, template.HTML("french"), h)
	h = frHTML("hello %s", "toto")
	assert.Equal(t, template.HTML("bonjour <strong>toto</strong>"), h)

	enHTML := TranslatorHTML("en", contextName)
	h = enHTML("english")
	assert.Equal(t, template.HTML("english"), h)
	h = enHTML("hello %s", "toto")
	assert.Equal(t, template.HTML("hello toto"), h)

	h = frHTML("context")
	assert.Equal(t, template.HTML("contexte<br />foo"), h)

	barHTML := TranslatorHTML("fr", "bar")
	h = barHTML("context")
	assert.Equal(t, template.HTML("contexte"), h)
}
