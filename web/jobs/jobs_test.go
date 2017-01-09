package jobs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
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

func TestCreateJobWithEventStream(t *testing.T) {
	body, _ := json.Marshal(&jobRequest{Arguments: "foobar"})
	req, err := http.NewRequest("POST", ts.URL+"/jobs/queue/print", bytes.NewReader(body))
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()
	r := bufio.NewReader(res.Body)
	events := []string{
		"queued",
		"running",
		"done",
	}
	var i int
	for {
		bs, err := r.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if !assert.NoError(t, err) {
		}
		if bytes.Equal(bs, []byte("\r\n")) {
			continue
		}
		spl := bytes.Split(bs, []byte(": "))
		if !assert.Len(t, spl, 2) {
			return
		}
		k, v := string(spl[0]), bytes.TrimSpace(spl[1])
		switch k {
		case "event":
			assert.Equal(t, events[i], string(v))
			i++
		case "data":
			var data *jobs.JobInfos
			assert.NoError(t, json.Unmarshal(v, &data))
		default:
			assert.Fail(t, "should not be here")
		}
		if i == len(events) {
			break
		}
	}
	assert.Equal(t, i, len(events))
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
