package jobs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

const host = "cozy.io"

var ts *httptest.Server

type jobRequest struct {
	Arguments interface{} `json:"arguments"`
}

func TestCreateJob(t *testing.T) {
	body, _ := json.Marshal(&jobRequest{Arguments: "foobar"})
	res, err := http.Post(ts.URL+"/jobs/queue/print", "application/json", bytes.NewReader(body))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 202, res.StatusCode)
}

func TestCreateJobNotExist(t *testing.T) {
	body, _ := json.Marshal(&jobRequest{Arguments: "foobar"})
	res, err := http.Post(ts.URL+"/jobs/queue/none", "application/json", bytes.NewReader(body))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 404, res.StatusCode)
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	instance.Destroy(host)

	inst, err := instance.Create(&instance.Options{
		Domain: host,
		Locale: "en",
	})
	if err != nil {
		fmt.Println("Could not create test instance.", err)
		os.Exit(1)
	}

	handler := echo.New()
	handler.HTTPErrorHandler = errors.ErrorHandler
	group := handler.Group("/jobs", injectInstance(inst))
	Routes(group)
	ts = httptest.NewServer(handler)

	res := m.Run()

	ts.Close()
	instance.Destroy(host)

	os.Exit(res)
}

func injectInstance(i *instance.Instance) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", i)
			return next(c)
		}
	}
}
