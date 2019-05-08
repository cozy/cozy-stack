package middlewares

import (
	"bytes"
	"html/template"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/echo"
)

// FuncsMap is a the helper functions used in templates.
// It is filled in web/statik but declared here to avoid circular imports.
var FuncsMap template.FuncMap

var cozyUITemplate *template.Template
var themeTemplate *template.Template
var faviconTemplate *template.Template

// BuildTemplates ensure that the cozy-ui can be injected in templates
func BuildTemplates() {
	cozyUITemplate = template.Must(template.New("cozy-ui").Funcs(FuncsMap).Parse(`` +
		`<link rel="stylesheet" type="text/css" href="{{asset .Domain "/css/cozy-ui.min.css" .ContextName}}">`,
	))
	themeTemplate = template.Must(template.New("theme").Funcs(FuncsMap).Parse(`` +
		`<link rel="stylesheet" type="text/css" href="{{asset .Domain "/styles/theme.css" .ContextName}}">`,
	))
	faviconTemplate = template.Must(template.New("favicon").Funcs(FuncsMap).Parse(`
	<link rel="icon" href="{{asset .Domain "/favicon.ico" .ContextName}}">
	<link rel="icon" type="image/png" href="{{asset .Domain "/favicon-16x16.png" .ContextName}}" sizes="16x16">
	<link rel="icon" type="image/png" href="{{asset .Domain "/favicon-32x32.png" .ContextName}}" sizes="32x32">
	<link rel="apple-touch-icon" sizes="180x180" href="{{asset .Domain "/apple-touch-icon.png" .ContextName}}"/>
		`))
}

// CozyUI returns an HTML template to insert the Cozy-UI assets.
func CozyUI(i *instance.Instance) template.HTML {
	buf := new(bytes.Buffer)
	err := cozyUITemplate.Execute(buf, echo.Map{
		"Domain":      i.ContextualDomain(),
		"ContextName": i.ContextName,
	})
	if err != nil {
		panic(err)
	}
	return template.HTML(buf.String())
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
	return template.HTML(buf.String())
}

// Favicon returns a helper to insert the favicons in an HTML template.
func Favicon(i *instance.Instance) template.HTML {
	buf := new(bytes.Buffer)
	err := faviconTemplate.Execute(buf, echo.Map{
		"Domain":      i.ContextualDomain(),
		"ContextName": i.ContextName,
	})
	if err != nil {
		panic(err)
	}
	return template.HTML(buf.String())
}
