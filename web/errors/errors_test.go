package errors

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrors_JSONAPI(t *testing.T) {
	tests := []struct {
		Name           string
		Error          error
		ExpectedStatus int
		ExpectedBody   string
	}{
		{
			Name: "with a couchdb error",
			Error: &couchdb.Error{
				StatusCode: 404,
				Name:       "some-error",
				Reason:     "some-details",
			},
			ExpectedStatus: 404,
			ExpectedBody:   `{"errors":[{"detail":"some-details","status":"404","title":"some-error","source":{}}]}`,
		},
		{
			Name:           "with an os.ErrExist error",
			Error:          os.ErrExist,
			ExpectedStatus: 409,
			ExpectedBody: `{"errors":[{"title":"Conflict","status":"409","detail":"file already exists","source":{}}]
      }`,
		},
		{
			Name:           "with an os.ErrNotExist error",
			Error:          os.ErrNotExist,
			ExpectedStatus: 404,
			ExpectedBody:   `{"errors":[{"title":"Not Found","status":"404","detail":"file does not exist","source":{}}]}`,
		},
		{
			Name:           "with an unexpected error",
			Error:          errors.New("unexpected-error"),
			ExpectedStatus: 500,
			ExpectedBody:   `{"errors":[{"title":"Unqualified error","status":"500","detail":"unexpected-error","source":{}}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()

			c := e.NewContext(req, rec)

			ErrorHandler(test.Error, c)

			assert.Equal(t, test.ExpectedStatus, rec.Code)
			assert.JSONEq(t, test.ExpectedBody, rec.Body.String())
		})
	}
}

func TestErrors_JSONAPI_With_HEAD_method(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)

	ErrorHandler(os.ErrNotExist, c)

	assert.Equal(t, 404, rec.Code)
	// Do not print the body with the Head method
	assert.Empty(t, rec.Body.String())
}

func TestErrors_do_nothing_on_committed_responses(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)

	c.NoContent(200)

	ErrorHandler(os.ErrNotExist, c)

	assert.Equal(t, 200, rec.Code)
	assert.Empty(t, req.Header.Get("Content-Type"))
	assert.Empty(t, rec.Body.String())
}

func TestErrors_instance_ErrNotFound_with_HTML(t *testing.T) {
	tests := []struct {
		Name                 string
		Error                error
		ExpectedStatus       int
		ExpectedBodyContains []string
	}{
		{
			Name:           "instance.ErrNotFound",
			Error:          instance.ErrNotFound,
			ExpectedStatus: 404,
			ExpectedBodyContains: []string{
				"Error Instance not found Title",
				"Error Instance not found Message",
				// Check that we are in inverted mode.
				"https://manager.cozycloud.cc/v2/cozy/remind",
				"Error Address forgotten",
				"/images/desert.svg",
			},
		},
		{
			Name:           "app.ErrNotFound",
			Error:          app.ErrNotFound,
			ExpectedStatus: 404,
			ExpectedBodyContains: []string{
				"Error Application not found Title",
				"Error Application not found Message",
				"/images/desert.svg",
			},
		},
		{
			Name:           "app.ErrInvalidSlugName",
			Error:          app.ErrInvalidSlugName,
			ExpectedStatus: 400,
			ExpectedBodyContains: []string{
				"Error Title",
				app.ErrInvalidSlugName.Error(),
			},
		},
		{
			Name: "any 4XX error use the generic error title and the error message",
			Error: &echo.HTTPError{
				Code:     400,
				Message:  "some-error",
				Internal: errors.New("some-internal-message"),
			},
			ExpectedStatus: 400,
			ExpectedBodyContains: []string{
				"Error Title",
				"some-error",
			},
		},
		{
			Name: "any 5XX error use the internal error title and message",
			Error: &echo.HTTPError{
				Code:     502,
				Message:  "some-error",
				Internal: errors.New("some-internal-message"),
			},
			ExpectedStatus: 502,
			ExpectedBodyContains: []string{
				"Error Internal Server Error Title",
				"Error Internal Server Error Message",
			},
		},
	}

	renderer, err := statik.NewDirRenderer("../../assets")
	require.NoError(t, err)

	middlewares.FuncsMap["asset"] = func(domain, name string, context ...string) string {
		return name
	}
	middlewares.BuildTemplates()

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			e := echo.New()
			e.Renderer = renderer
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Add("Accept", echo.MIMETextHTML)

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			HTMLErrorHandler(test.Error, c)

			assert.Equal(t, test.ExpectedStatus, rec.Code)
			assert.Equal(t, echo.MIMETextHTMLCharsetUTF8, rec.Header().Get("Content-Type"))

			for _, expected := range test.ExpectedBodyContains {
				assert.Contains(t, rec.Body.String(), expected)
			}
		})
	}
}

func TestErrors_HTMLError_With_HEAD_method(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodHead, "/", nil)
	req.Header.Add("Accept", echo.MIMETextHTML)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)

	HTMLErrorHandler(&echo.HTTPError{
		Code:     400,
		Message:  "some-error",
		Internal: errors.New("some-internal-message"),
	}, c)

	assert.Equal(t, 400, rec.Code)
	// Do not print the body with the Head method
	assert.Empty(t, rec.Body.String())
}

func TestErrors_with_no_Accept_header(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)

	HTMLErrorHandler(&echo.HTTPError{
		Code:     400,
		Message:  "some-error",
		Internal: errors.New("some-internal-message"),
	}, c)

	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "some-error", rec.Body.String())
}

func TestErrors_HTMLError_with_an_JSON_Accept_header(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept", echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)

	ErrorHandler(&echo.HTTPError{
		Code:     400,
		Message:  "some-error",
		Internal: errors.New("some-internal-message"),
	}, c)

	assert.Equal(t, 400, rec.Code)
	assert.JSONEq(t, `{ "error": "some-error" }`, rec.Body.String())
}
