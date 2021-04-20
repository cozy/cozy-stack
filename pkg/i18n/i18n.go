package i18n

import (
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/goodsign/monday"
	"github.com/leonelquinteros/gotext"
)

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

var boldRegexp = regexp.MustCompile(`\*\*(.*)\*\*`)

// TranslatorHTML returns a translation function of the locale specified, which
// allow simple markup like **bold**.
func TranslatorHTML(locale string) func(key string, vars ...interface{}) template.HTML {
	return func(key string, vars ...interface{}) template.HTML {
		translated := Translate(key, locale, vars...)
		escaped := template.HTMLEscapeString(translated)
		replaced := boldRegexp.ReplaceAllString(escaped, "<strong>$1</strong>")
		return template.HTML(replaced)
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
	if po, ok := translations[consts.DefaultLocale]; ok {
		translated := po.Get(key, vars...)
		if translated != key && translated != "" {
			return translated
		}
	}
	logger.WithNamespace("i18n").
		Infof("Translation not found for key %q on locale %q", key, locale)
	if strings.HasPrefix(key, " Permissions ") {
		key = strings.Replace(key, "Permissions ", "", 1)
	}
	return fmt.Sprintf(key, vars...)
}

// LocalizeTime transforms a date+time in a string for the given locale.
// The layout is in the same format as the one given to time.Format.
func LocalizeTime(t time.Time, locale, layout string) string {
	return monday.Format(t, layout, mondayLocale(locale))
}

func mondayLocale(locale string) monday.Locale {
	switch locale {
	case "de", "de_DE":
		return monday.LocaleDeDE
	case "es", "es_ES":
		return monday.LocaleEsES
	case "fr", "fr_FR":
		return monday.LocaleFrFR
	case "it", "it_IT":
		return monday.LocaleItIT
	case "ja", "ja_JP":
		return monday.LocaleJaJP
	case "nl", "nl_NL":
		return monday.LocaleNlNL
	case "pt", "pt_PT":
		return monday.LocalePtPT
	case "ru", "ru_RU":
		return monday.LocaleRuRU
	default:
		return monday.LocaleEnUS
	}
}
