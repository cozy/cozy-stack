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
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
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

func TestAddGetAndDeleteTriggerAt(t *testing.T) {
	at := time.Now().Add(1 * time.Second).Format(time.RFC3339)
	body, _ := json.Marshal(map[string]interface{}{
		"type":             "@at",
		"arguments":        at,
		"worker":           "print",
		"worker_arguments": "foo",
	})
	res1, err := http.Post(ts.URL+"/jobs/triggers", "application/json", bytes.NewReader(body))
	if !assert.NoError(t, err) {
		return
	}
	defer res1.Body.Close()
	assert.Equal(t, http.StatusCreated, res1.StatusCode)

	var v struct {
		Data struct {
			ID         string             `json:"id"`
			Type       string             `json:"type"`
			Attributes *jobs.TriggerInfos `json:"attributes"`
		}
	}
	err = json.NewDecoder(res1.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}

	triggerID := v.Data.ID
	assert.Equal(t, consts.Triggers, v.Data.Type)
	assert.Equal(t, "@at", v.Data.Attributes.Type)
	assert.Equal(t, at, v.Data.Attributes.Arguments)
	assert.Equal(t, "print", v.Data.Attributes.WorkerType)

	body, _ = json.Marshal(map[string]interface{}{
		"type":             "@at",
		"arguments":        "garbage",
		"worker":           "print",
		"worker_arguments": "foo",
	})
	res2, err := http.Post(ts.URL+"/jobs/triggers", "application/json", bytes.NewReader(body))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusBadRequest, res2.StatusCode)

	res3, err := http.Get(ts.URL + "/jobs/triggers/" + triggerID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res3.StatusCode)

	req4, err := http.NewRequest("DELETE", ts.URL+"/jobs/triggers/"+triggerID, nil)
	if !assert.NoError(t, err) {
		return
	}

	res4, err := http.DefaultClient.Do(req4)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res4.StatusCode)

	res5, err := http.Get(ts.URL + "/jobs/triggers/" + triggerID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusNotFound, res5.StatusCode)
}

func TestAddGetAndDeleteTriggerIn(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"type":             "@in",
		"arguments":        "1s",
		"worker":           "print",
		"worker_arguments": "foo",
	})
	res1, err := http.Post(ts.URL+"/jobs/triggers", "application/json", bytes.NewReader(body))
	if !assert.NoError(t, err) {
		return
	}
	defer res1.Body.Close()
	assert.Equal(t, http.StatusCreated, res1.StatusCode)

	var v struct {
		Data struct {
			ID         string             `json:"id"`
			Type       string             `json:"type"`
			Attributes *jobs.TriggerInfos `json:"attributes"`
		}
	}
	err = json.NewDecoder(res1.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}

	triggerID := v.Data.ID
	assert.Equal(t, consts.Triggers, v.Data.Type)
	assert.Equal(t, "@in", v.Data.Attributes.Type)
	assert.Equal(t, "1s", v.Data.Attributes.Arguments)
	assert.Equal(t, "print", v.Data.Attributes.WorkerType)

	body, _ = json.Marshal(map[string]interface{}{
		"type":             "@in",
		"arguments":        "garbage",
		"worker":           "print",
		"worker_arguments": "foo",
	})
	res2, err := http.Post(ts.URL+"/jobs/triggers", "application/json", bytes.NewReader(body))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusBadRequest, res2.StatusCode)

	res3, err := http.Get(ts.URL + "/jobs/triggers/" + triggerID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res3.StatusCode)

	req4, err := http.NewRequest("DELETE", ts.URL+"/jobs/triggers/"+triggerID, nil)
	if !assert.NoError(t, err) {
		return
	}

	res4, err := http.DefaultClient.Do(req4)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res4.StatusCode)

	res5, err := http.Get(ts.URL + "/jobs/triggers/" + triggerID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusNotFound, res5.StatusCode)
}

func TestGetAllJobs(t *testing.T) {
	var v struct {
		Data []struct {
			ID         string             `json:"id"`
			Type       string             `json:"type"`
			Attributes *jobs.TriggerInfos `json:"attributes"`
		}
	}

	res1, err := http.Get(ts.URL + "/jobs/triggers")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res1.StatusCode)

	err = json.NewDecoder(res1.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}

	assert.Len(t, v.Data, 0)

	body, _ := json.Marshal(map[string]interface{}{
		"type":             "@interval",
		"arguments":        "1s",
		"worker":           "print",
		"worker_arguments": "foo",
	})
	_, err = http.Post(ts.URL+"/jobs/triggers", "application/json", bytes.NewReader(body))
	if !assert.NoError(t, err) {
		return
	}

	res3, err := http.Get(ts.URL + "/jobs/triggers")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res3.StatusCode)

	err = json.NewDecoder(res3.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}

	if assert.Len(t, v.Data, 1) {
		assert.Equal(t, consts.Triggers, v.Data[0].Type)
		assert.Equal(t, "@interval", v.Data[0].Attributes.Type)
		assert.Equal(t, "1s", v.Data[0].Attributes.Arguments)
		assert.Equal(t, "print", v.Data[0].Attributes.WorkerType)
	}
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

	if err = inst.StartJobSystem(); err != nil {
		fmt.Println(err)
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
