package middlewares

import (
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/echo"

	"github.com/golang/gddo/httputil"
)

const (
	defaultContentTypeOffer = jsonapi.ContentType
	acceptContentTypeKey    = "offer-content-type"
)

var contentTypeOffers = []string{
	jsonapi.ContentType,
	echo.MIMEApplicationJSON,
	echo.MIMETextHTML,
	echo.MIMETextPlain,
}

// AcceptOptions can be used to parameterize the the Accept middleware: the
// default content-type in case no offer is accepted, and the list of offers to
// select from.
type AcceptOptions struct {
	DefaultContentTypeOffer string
	Offers                  []string
}

// AcceptJSON is an echo middleware that checks that the HTTP Accept header
// is compatible with application/json
func AcceptJSON(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		accept := c.Request().Header.Get(echo.HeaderAccept)
		if strings.Contains(accept, echo.MIMEApplicationJSON) {
			return next(c)
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "bad_accept_header",
		})
	}
}

// ContentTypeJSON is an echo middleware that checks that the HTTP Content-Type
// header is compatible with application/json
func ContentTypeJSON(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		contentType := c.Request().Header.Get(echo.HeaderContentType)
		if strings.HasPrefix(contentType, echo.MIMEApplicationJSON) {
			return next(c)
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "bad_content_type",
		})
	}
}

// Accept is a middleware resolving the better content-type offering for the
// HTTP request, given the `Accept` header and the middleware options.
func Accept(args ...AcceptOptions) echo.MiddlewareFunc {
	var opts AcceptOptions
	if len(args) > 0 {
		opts = args[0]
	}
	if opts.DefaultContentTypeOffer == "" {
		opts.DefaultContentTypeOffer = defaultContentTypeOffer
	}
	if opts.Offers == nil {
		opts.Offers = contentTypeOffers
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			contentTypeOffer := httputil.NegotiateContentType(c.Request(), opts.Offers, opts.DefaultContentTypeOffer)
			c.Set(acceptContentTypeKey, contentTypeOffer)
			return next(c)
		}
	}
}

// AcceptedContentType returns the accepted content-type store from the Accept
// middleware.
func AcceptedContentType(c echo.Context) string {
	return c.Get(acceptContentTypeKey).(string)
}
