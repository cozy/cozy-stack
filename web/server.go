//go:generate statik -src=../assets

// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
//
// Terms Of Service:
//
// there are no TOS at this moment, use at your own risk we take no responsibility
//
//     Schemes: https
//     Host: localhost
//     BasePath: /
//     Version: 0.0.1
//     License: AGPL-3.0 https://opensource.org/licenses/agpl-3.0
//     Contact: Bruno Michel <bruno@cozycloud.cc> https://cozy.io/
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
// swagger:meta
package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
	_ "github.com/cozy/cozy-stack/web/statik" // Generated file with the packed assets
	"github.com/labstack/echo"
	"github.com/rakyll/statik/fs"
)

var templatesList = []string{
	"login.html",
}

// Config represents the configuration
type Config struct {
	Router    *echo.Echo
	Domain    string
	Assets    string
	ServeApps func(c echo.Context, domain, slug string) error
}

type renderer struct {
	t *template.Template
	h http.Handler
}

func (r *renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.t.ExecuteTemplate(w, name, data)
}

func (r *renderer) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	r.h.ServeHTTP(res, req)
}

func createRenderer(conf *Config) (*renderer, error) {
	// By default, use the assets packed in the binary
	if conf.Assets != "" {
		if !path.IsAbs(conf.Assets) {
			return nil, fmt.Errorf("path %s is not absolute", conf.Assets)
		}
		list := make([]string, len(templatesList))
		for i, name := range templatesList {
			list[i] = path.Join(conf.Assets, "templates", name)
		}
		t, err := template.ParseFiles(list...)
		if err != nil {
			return nil, err
		}
		h := http.FileServer(http.Dir(conf.Assets))
		r := &renderer{t, h}
		return r, nil
	}

	statikFS, err := fs.New()
	if err != nil {
		return nil, err
	}

	var t, tmpl *template.Template
	for _, name := range templatesList {
		if t == nil {
			t = template.New(name)
			tmpl = t
		} else {
			tmpl = t.New(name)
		}
		f, err := statikFS.Open("/templates/" + name)
		if err != nil {
			return nil, err
		}
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}
		_, err = tmpl.Parse(string(b))
		if err != nil {
			return nil, err
		}
	}

	h := http.FileServer(statikFS)
	r := &renderer{t, h}
	return r, nil
}

func slugFromHost(host, domain string) (parent, slug string) {
	parts := strings.SplitN(host, ".", 3)
	if len(parts) != 3 {
		return host, ""
	}
	if parts[2] != domain {
		return host, ""
	}
	return parts[1] + "." + parts[2], parts[0]
}

// Create returns a new web server that will handle that apps routing given the
// host of the request. It also adds that the asset handler in /assets/ as well
// as a template rendering to use c.Render.
func Create(conf *Config) (*echo.Echo, error) {
	appsRouter := echo.New()
	apisRouter := conf.Router

	r, err := createRenderer(conf)
	if err != nil {
		return nil, err
	}

	apisRouter.GET("/assets/*", echo.WrapHandler(http.StripPrefix("/assets/", r)))
	apisRouter.Renderer = r
	apisRouter.HTTPErrorHandler = ErrorHandler

	appsRouter.Any("/*", func(c echo.Context) error {
		req := c.Request()
		parent, slug := slugFromHost(req.Host, conf.Domain)
		if slug == "" {
			apisRouter.ServeHTTP(c.Response(), req)
			return nil
		}
		if conf.ServeApps != nil {
			return conf.ServeApps(c, parent, slug)
		}
		return nil
	})

	return appsRouter, nil
}

// ErrorHandler is the default error handler of our server. It always write a
// jsonapi compatible error.
func ErrorHandler(err error, c echo.Context) {
	var je *jsonapi.Error
	var ce *couchdb.Error
	var he *echo.HTTPError
	var ok bool

	res := c.Response()
	req := c.Request()

	if he, ok = err.(*echo.HTTPError); ok {
		if !res.Committed {
			if c.Request().Method == http.MethodHead {
				c.NoContent(he.Code)
			} else {
				c.String(he.Code, he.Message)
			}
		}
		if config.IsDevRelease() {
			log.Errorf("[HTTP %s %s] %s", req.Method, req.URL.Path, err)
		}
		return
	}

	if ce, ok = err.(*couchdb.Error); ok {
		je = &jsonapi.Error{
			Status: ce.StatusCode,
			Title:  ce.Name,
			Detail: ce.Reason,
		}
	} else if je, ok = err.(*jsonapi.Error); !ok {
		je = &jsonapi.Error{
			Status: http.StatusInternalServerError,
			Title:  "Unqualified error",
			Detail: err.Error(),
		}
	}

	if !res.Committed {
		if c.Request().Method == http.MethodHead {
			c.NoContent(je.Status)
		} else {
			res.Header().Set("Content-Type", jsonapi.ContentType)
			res.WriteHeader(je.Status)
			json.NewEncoder(res).Encode(je)
		}
	}

	if config.IsDevRelease() {
		log.Errorf("[HTTP %s %s] %s", req.Method, req.URL.Path, err)
	}
}
