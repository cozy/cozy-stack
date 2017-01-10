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

type event struct {
	name string
	data []byte
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
	evch := make(chan *event, 1)

	go func() {
		err := parseEventStream(r, evch)
		assert.NoError(t, err)
	}()

	var i int
	for ev := range evch {
		var data *jobs.JobInfos
		assert.Equal(t, events[i], ev.name)
		if assert.NotNil(t, ev.data) {
			assert.NoError(t, json.Unmarshal(ev.data, &data))
		}
		fmt.Println(ev.name, data)
		i++
	}
	assert.Equal(t, i, len(events))
}

func parseEventStream(r *bufio.Reader, evch chan *event) error {
	defer close(evch)
	var ev *event
	for {
		bs, err := r.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if bytes.Equal(bs, []byte("\r\n")) {
			ev = nil
			continue
		}
		spl := bytes.Split(bs, []byte(": "))
		if len(spl) != 2 {
			return fmt.Errorf("should be length 2")
		}
		k, v := string(spl[0]), bytes.TrimSpace(spl[1])
		switch k {
		case "event":
			ev = &event{name: string(v)}
		case "data":
			if ev == nil {
				return fmt.Errorf("could not parse event stream")
			}
			ev.data = v
			evch <- ev
		default:
			return fmt.Errorf("should not be here")
		}
	}
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
