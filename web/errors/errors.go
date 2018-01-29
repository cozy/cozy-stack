package errors

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"

	"github.com/golang/gddo/httputil"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
)

var contentTypeOffers = []string{
	jsonapi.ContentType,
	echo.MIMEApplicationJSON,
	echo.MIMETextHTML,
	echo.MIMETextPlain,
}

const defaultContentTypeOffer = jsonapi.ContentType

// DefaultContentTypeOfferKey is a key for the echo.Context that can be used
// to set a default content-type offer for the error response.
const DefaultContentTypeOfferKey = "default-content-type"

// ErrorNormalized is created by the error handler to normalize any error into
// a struct containing all the elements to create a full HTTP error response.
type ErrorNormalized struct {
	status       int
	title        string
	detail       string
	titleLocale  string
	detailLocale string
	inner        error
}

// ToJSONAPI return a *jsonapi.Error created from the normalized error
func (e *ErrorNormalized) ToJSONAPI() *jsonapi.Error {
	return &jsonapi.Error{
		Status: e.Status(),
		Title:  e.Title(),
		Detail: e.Detail(),
	}
}

// Status return the HTTP status code associated with the normalized error.
func (e *ErrorNormalized) Status() int {
	return e.status
}

// Title returns the error title string value.
func (e *ErrorNormalized) Title() string {
	if e.title != "" {
		return e.title
	}
	return e.inner.Error()
}

// Detail returns the error detailed string value.
func (e *ErrorNormalized) Detail() string {
	if e.detail != "" {
		return e.detail
	}
	return e.inner.Error()
}

// TitleLocale returns the locale code of the title of the error is any, and
// the title if none.
func (e *ErrorNormalized) TitleLocale() string {
	if e.titleLocale != "" {
		return e.titleLocale
	}
	return e.Title()
}

// DetailLocale returns the locale code of the detail of the error is any, and
// the detail if none.
func (e *ErrorNormalized) DetailLocale() string {
	if e.detailLocale != "" {
		return e.detailLocale
	}
	return e.Detail()
}

// NormalizeError creates a normalized version of the given error that can be
// used to create an HTTP response.
func NormalizeError(err error) *ErrorNormalized {
	if err == nil {
		return nil
	}

	n := ErrorNormalized{
		inner:  err,
		status: http.StatusInternalServerError,
	}

	if he, ok := err.(*echo.HTTPError); ok {
		n.status = he.Code
		if he.Inner != nil {
			err = he.Inner
		} else {
			n.detail = fmt.Sprintf("%v", he.Message)
		}
	}

	if os.IsExist(err) {
		n.status = http.StatusConflict
	} else if os.IsNotExist(err) {
		n.status = http.StatusNotFound
	} else if err == instance.ErrNotFound {
		n.status = http.StatusNotFound
		n.titleLocale = "Error Instance not found Title"
		n.detailLocale = "Error Instance not found Message"
	} else if je, ok := err.(*jsonapi.Error); ok {
		n.status = je.Status
		n.title = je.Title
		n.detail = je.Detail
	} else if ce, ok := err.(*couchdb.Error); ok {
		n.status = ce.StatusCode
		n.title = ce.Name
		n.detail = ce.Reason
	}

	if n.title == "" {
		n.title = http.StatusText(n.status)
		if n.status >= http.StatusInternalServerError {
			n.titleLocale = "Error Internal Server Error Title"
			n.detailLocale = "Error Internal Server Error Message"
		} else {
			n.titleLocale = "Error Title"
		}
	}

	if n.detail == "" {
		n.detail = err.Error()
	}

	return &n
}

// ErrorHandler is the default error handler of our APIs.
func ErrorHandler(err error, c echo.Context) {
	WriteError(err, c.Response(), c)
}

// WriteError can be used to write an error response in a specific
// http.ResponseWriter different than the echo.Content response. It is
// particularly useful for hijacked responses.
func WriteError(err error, res http.ResponseWriter, c echo.Context) {
	req := c.Request()
	inst, _ := middlewares.GetInstanceSafe(c)

	var log *logrus.Entry
	if config.IsDevRelease() {
		if inst != nil {
			log = inst.Logger()
		} else {
			log = logger.WithNamespace("http")
		}
		log.Errorf("[http] %s %s %s", req.Method, req.URL.Path, err)
	}

	if c.Response().Committed {
		return
	}

	wantedContentTypeOffer := defaultContentTypeOffer
	if s, ok := c.Get(DefaultContentTypeOfferKey).(string); ok {
		wantedContentTypeOffer = s
	}

	errn := NormalizeError(err)
	contentTypeOffer := httputil.NegotiateContentType(req, contentTypeOffers, wantedContentTypeOffer)

	b := bufferPool.Get().(*bytes.Buffer)
	defer func() {
		b.Reset()
		bufferPool.Put(b)
	}()

	var contentType string
	switch contentTypeOffer {
	case jsonapi.ContentType, echo.MIMEApplicationJSON:
		contentType = jsonapi.ContentType
		jsonapi.WriteError(b, errn.ToJSONAPI())
	case echo.MIMETextHTML:
		contentType = echo.MIMETextHTML
		var domain string
		if inst != nil {
			domain = inst.Domain
		}
		c.Echo().Renderer.Render(b, "error.html", echo.Map{
			"Domain":     domain,
			"ErrorTitle": errn.TitleLocale(),
			"Error":      errn.DetailLocale(),
		}, c)
	case echo.MIMETextPlain:
		contentType = echo.MIMETextPlain
		title, detail := errn.Title(), errn.Detail()
		if detail != "" {
			fmt.Fprintf(b, "%s: %s", title, detail)
		} else {
			b.WriteString(title)
		}
	}

	res.Header().Set("Content-Type", contentType)
	res.Header().Set("Content-Length", strconv.Itoa(b.Len()))
	res.WriteHeader(errn.Status())
	res.Write(b.Bytes())
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}
