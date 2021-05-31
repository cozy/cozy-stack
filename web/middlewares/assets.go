package middlewares

import (
	"bytes"
	"html/template"

	"github.com/cozy/cozy-stack/model/instance"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/labstack/echo/v4"
)

// FuncsMap is a the helper functions used in templates.
// It is filled in web/statik but declared here to avoid circular imports.
var FuncsMap template.FuncMap

var cozyBSTemplate *template.Template
var cozyUITemplate *template.Template
var themeTemplate *template.Template
var faviconTemplate *template.Template

// BuildTemplates ensure that the cozy-ui can be injected in templates
func BuildTemplates() {
	cozyBSTemplate = template.Must(template.New("cozy-bs").Funcs(FuncsMap).Parse(`` +
		`<link rel="stylesheet" type="text/css" href="{{asset .Domain "/css/cozy-bs.min.css" .ContextName}}">`,
	))
	cozyUITemplate = template.Must(template.New("cozy-ui").Funcs(FuncsMap).Parse(`` +
		`<link rel="stylesheet" type="text/css" href="{{asset .Domain "/css/cozy-ui.min.css" .ContextName}}">`,
	))
	themeTemplate = template.Must(template.New("theme").Funcs(FuncsMap).Parse(`` +
		`<link rel="stylesheet" type="text/css" href="{{asset .Domain "/styles/theme.css" .ContextName}}">`,
	))
	faviconTemplate = template.Must(template.New("favicon").Funcs(FuncsMap).Parse(`
	<link rel="icon" href="{{asset .Domain "/favicon.ico" .ContextName}}">
	{{if .Dev}}
	<link rel="icon" href="{{asset .Domain "/images/cozy-dev.svg"}}" type="image/svg+xml" sizes="any">
	{{else}}
	<link rel="icon" href="{{asset .Domain "/icon.svg"}}" type="image/svg+xml" sizes="any">
	{{end}}
	<link rel="apple-touch-icon" sizes="180x180" href="{{asset .Domain "/apple-touch-icon.png" .ContextName}}"/>
	<link rel="manifest" href="{{asset .Domain "/manifest.webmanifest"}}">
		`))
}

// CozyBS returns an HTML template to insert the Cozy-bootstrap assets.
func CozyBS(i *instance.Instance) template.HTML {
	buf := new(bytes.Buffer)
	err := cozyBSTemplate.Execute(buf, echo.Map{
		"Domain":      i.ContextualDomain(),
		"ContextName": i.ContextName,
	})
	if err != nil {
		panic(err)
	}
	return template.HTML(buf.String())
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
		"Dev":         build.IsDevRelease(),
	})
	if err != nil {
		panic(err)
	}
	return template.HTML(buf.String())
}
