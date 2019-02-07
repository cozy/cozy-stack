package middlewares

import (
	"bytes"
	"html/template"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/echo"
)

// FuncsMap is a the helper functions used in templates.
// It is filled in wen/statik but declared here to avoid circular imports.
var FuncsMap template.FuncMap

var cozyUITemplate *template.Template
var themeTemplate *template.Template

// BuildTemplates ensure that the cozy-ui can be injected in templates
func BuildTemplates() {
	cozyUITemplate = template.Must(template.New("cozy-ui").Funcs(FuncsMap).Parse(`` +
		`<link rel="stylesheet" type="text/css" href="{{asset .Domain "/css/cozy-ui.min.css" .ContextName}}">`,
	))
	themeTemplate = template.Must(template.New("theme").Funcs(FuncsMap).Parse(`` +
		`<link rel="stylesheet" type="text/css" href="{{asset .Domain "/styles/theme.css" .ContextName}}">`,
	))
}

func CozyUI(i *instance.Instance) template.HTML {
	buf := new(bytes.Buffer)
	err := cozyUITemplate.Execute(buf, echo.Map{
		"Domain":      i.ContextualDomain(),
		"ContextName": i.ContextName,
	})
	if err != nil {
		panic(err)
	}
	return template.HTML(buf.String()) // #nosec
}

// ThemeCSS returns an HTML template for inserting the HTML tag for the custom
// CSS theme
func ThemeCSS(i *instance.Instance) template.HTML {
	buf := new(bytes.Buffer)
	err := themeTemplate.Execute(buf, echo.Map{
		"Domain":      i.ContextualDomain(),
		"ContextName": i.ContextName,
	})
	if err != nil {
		panic(err)
	}
	return template.HTML(buf.String()) // #nosec
}
