package i18n

import (
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/pkg/logger"
	gotext "gopkg.in/leonelquinteros/gotext.v1"
)

// DefaultLocale is the default locale tag, used for fallback.
const DefaultLocale = "en"

// SupportedLocales is the list of supported locales tags.
var SupportedLocales = []string{"en", "fr"}

var translations = make(map[string]*gotext.Po)

// LoadLocale creates the translation object for a locale from the content of a .po file
func LoadLocale(identifier string, rawPO []byte) {
	po := &gotext.Po{Language: identifier}
	po.Parse(rawPO)
	translations[identifier] = po
}

// Translator returns a translation function of the locale specified
func Translator(locale string) func(key string, vars ...interface{}) string {
	return func(key string, vars ...interface{}) string {
		return Translate(key, locale, vars...)
	}
}

// Translate translates the given key on the specified locale.
func Translate(key, locale string, vars ...interface{}) string {
	if po, ok := translations[locale]; ok {
		translated := po.Get(key, vars...)
		if translated != key && translated != "" {
			return translated
		}
	}
	if po, ok := translations[DefaultLocale]; ok {
		translated := po.Get(key, vars...)
		if translated != key && translated != "" {
			return translated
		}
	}
	logger.WithNamespace("i18n").Infof("Translation not found for key %q on locale %q", key, locale)
	if strings.HasPrefix(key, " Permissions ") {
		key = strings.Replace(key, "Permissions ", "", 1)
	}
	return fmt.Sprintf(key, vars...)
}
