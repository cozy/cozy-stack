package jobs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var testInstance *instance.Instance
var token string

type jobRequest struct {
	Arguments interface{} `json:"arguments"`
}

type jsonapiReq struct {
	Data *jsonapiData `json:"data"`
}

type jsonapiData struct {
	Attributes interface{} `json:"attributes"`
}

func TestGetQueue(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/queue/print", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	assert.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data := result["data"].([]interface{})
	assert.Equal(t, 0, len(data))
}

func TestCreateJob(t *testing.T) {
	body, _ := json.Marshal(&jsonapiReq{
		Data: &jsonapiData{
			Attributes: &jobRequest{Arguments: "foobar"},
		},
	})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/queue/print", bytes.NewReader(body))
	req.Header.Add("Authorization", "Bearer "+token)
	assert.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 202, res.StatusCode)
}

func TestCreateJobNotExist(t *testing.T) {
	body, _ := json.Marshal(&jsonapiReq{
		Data: &jsonapiData{
			Attributes: &jobRequest{Arguments: "foobar"},
		},
	})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/queue/none", bytes.NewReader(body))
	tokenNone, _ := testInstance.MakeJWT(consts.CLIAudience, "CLI",
		consts.Jobs+":ALL:none:worker",
		"", time.Now())
	req.Header.Add("Authorization", "Bearer "+tokenNone)
	assert.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 404, res.StatusCode)
}

func TestAddGetAndDeleteTriggerAt(t *testing.T) {
	at := time.Now().Add(1100 * time.Millisecond).Format(time.RFC3339)
	body, _ := json.Marshal(&jsonapiReq{
		Data: &jsonapiData{
			Attributes: &map[string]interface{}{
				"type":      "@at",
				"arguments": at,
				"worker":    "print",
				"message":   "foo",
			},
		},
	})
	req1, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/triggers", bytes.NewReader(body))
	assert.NoError(t, err)
	req1.Header.Add("Authorization", "Bearer "+token)
	res1, err := http.DefaultClient.Do(req1)
	if !assert.NoError(t, err) {
		return
	}
	defer res1.Body.Close()
	assert.Equal(t, http.StatusCreated, res1.StatusCode)

	var v struct {
		Data struct {
			ID         string            `json:"id"`
			Type       string            `json:"type"`
			Attributes *job.TriggerInfos `json:"attributes"`
		}
	}
	err = json.NewDecoder(res1.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.NotNil(t, v.Data) || !assert.NotNil(t, v.Data.Attributes) {
		return
	}
	triggerID := v.Data.ID
	assert.Equal(t, consts.Triggers, v.Data.Type)
	assert.Equal(t, "@at", v.Data.Attributes.Type)
	assert.Equal(t, at, v.Data.Attributes.Arguments)
	assert.Equal(t, "print", v.Data.Attributes.WorkerType)

	body, _ = json.Marshal(&jsonapiReq{
		Data: &jsonapiData{
			Attributes: map[string]interface{}{
				"type":      "@at",
				"arguments": "garbage",
				"worker":    "print",
				"message":   "foo",
			},
		},
	})
	req2, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/triggers", bytes.NewReader(body))
	assert.NoError(t, err)
	req2.Header.Add("Authorization", "Bearer "+token)
	res2, err := http.DefaultClient.Do(req2)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusBadRequest, res2.StatusCode)

	req3, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
	assert.NoError(t, err)
	req3.Header.Add("Authorization", "Bearer "+token)
	res3, err := http.DefaultClient.Do(req3)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res3.StatusCode)

	req4, err := http.NewRequest("DELETE", ts.URL+"/jobs/triggers/"+triggerID, nil)
	assert.NoError(t, err)
	req4.Header.Add("Authorization", "Bearer "+token)
	res4, err := http.DefaultClient.Do(req4)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusNoContent, res4.StatusCode)

	req5, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
	assert.NoError(t, err)
	req5.Header.Add("Authorization", "Bearer "+token)
	res5, err := http.DefaultClient.Do(req5)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusNotFound, res5.StatusCode)
}

func TestAddGetAndDeleteTriggerIn(t *testing.T) {
	body, _ := json.Marshal(&jsonapiReq{
		Data: &jsonapiData{
			Attributes: map[string]interface{}{
				"type":      "@in",
				"arguments": "1s",
				"worker":    "print",
				"message":   "foo",
			},
		},
	})
	req1, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/triggers", bytes.NewReader(body))
	assert.NoError(t, err)
	req1.Header.Add("Authorization", "Bearer "+token)
	res1, err := http.DefaultClient.Do(req1)
	if !assert.NoError(t, err) {
		return
	}
	defer res1.Body.Close()
	assert.Equal(t, http.StatusCreated, res1.StatusCode)

	var v struct {
		Data struct {
			ID         string            `json:"id"`
			Type       string            `json:"type"`
			Attributes *job.TriggerInfos `json:"attributes"`
		}
	}
	err = json.NewDecoder(res1.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.NotNil(t, v.Data) || !assert.NotNil(t, v.Data.Attributes) {
		return
	}
	triggerID := v.Data.ID
	assert.Equal(t, consts.Triggers, v.Data.Type)
	assert.Equal(t, "@in", v.Data.Attributes.Type)
	assert.Equal(t, "1s", v.Data.Attributes.Arguments)
	assert.Equal(t, "print", v.Data.Attributes.WorkerType)

	body, _ = json.Marshal(&jsonapiReq{
		Data: &jsonapiData{
			Attributes: map[string]interface{}{
				"type":      "@in",
				"arguments": "garbage",
				"worker":    "print",
				"message":   "foo",
			},
		},
	})
	req2, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/triggers", bytes.NewReader(body))
	assert.NoError(t, err)
	req2.Header.Add("Authorization", "Bearer "+token)
	res2, err := http.DefaultClient.Do(req2)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusBadRequest, res2.StatusCode)

	req3, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
	assert.NoError(t, err)
	req3.Header.Add("Authorization", "Bearer "+token)
	res3, err := http.DefaultClient.Do(req3)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res3.StatusCode)

	req4, err := http.NewRequest("DELETE", ts.URL+"/jobs/triggers/"+triggerID, nil)
	assert.NoError(t, err)
	req4.Header.Add("Authorization", "Bearer "+token)
	res4, err := http.DefaultClient.Do(req4)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusNoContent, res4.StatusCode)

	req5, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
	assert.NoError(t, err)
	req5.Header.Add("Authorization", "Bearer "+token)
	res5, err := http.DefaultClient.Do(req5)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusNotFound, res5.StatusCode)
}

func TestGetAllJobs(t *testing.T) {
	var v struct {
		Data []struct {
			ID         string            `json:"id"`
			Type       string            `json:"type"`
			Attributes *job.TriggerInfos `json:"attributes"`
		}
	}

	req1, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers", nil)
	assert.NoError(t, err)
	tokenTriggers, _ := testInstance.MakeJWT(consts.CLIAudience, "CLI", consts.Triggers, "", time.Now())
	req1.Header.Add("Authorization", "Bearer "+tokenTriggers)
	res1, err := http.DefaultClient.Do(req1)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res1.StatusCode)

	err = json.NewDecoder(res1.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}

	// The instance already has a trigger for thumbnails
	assert.Len(t, v.Data, 1)

	body, _ := json.Marshal(&jsonapiReq{
		Data: &jsonapiData{
			Attributes: map[string]interface{}{
				"type":      "@in",
				"arguments": "10s",
				"worker":    "print",
				// worker_arguments is deprecated but should still works
				// we are using it here to check that it still works
				"worker_arguments": "foo",
			},
		},
	})
	req2, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/triggers", bytes.NewReader(body))
	assert.NoError(t, err)
	req2.Header.Add("Authorization", "Bearer "+token)
	res2, err := http.DefaultClient.Do(req2)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusCreated, res2.StatusCode)

	req3, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers", nil)
	assert.NoError(t, err)
	req3.Header.Add("Authorization", "Bearer "+tokenTriggers)
	res3, err := http.DefaultClient.Do(req3)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res3.StatusCode)

	err = json.NewDecoder(res3.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}

	if assert.Len(t, v.Data, 2) {
		var index int
		if v.Data[1].Attributes.Type == "@in" {
			index = 1
		} else {
			index = 0
		}
		assert.Equal(t, consts.Triggers, v.Data[index].Type)
		assert.Equal(t, "@in", v.Data[index].Attributes.Type)
		assert.Equal(t, "10s", v.Data[index].Attributes.Arguments)
		assert.Equal(t, "print", v.Data[index].Attributes.WorkerType)
	}

	req4, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers?Worker=print", nil)
	assert.NoError(t, err)
	req4.Header.Add("Authorization", "Bearer "+tokenTriggers)
	res4, err := http.DefaultClient.Do(req4)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res4.StatusCode)

	err = json.NewDecoder(res4.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}

	if assert.Len(t, v.Data, 1) {
		assert.Equal(t, consts.Triggers, v.Data[0].Type)
		assert.Equal(t, "@in", v.Data[0].Attributes.Type)
		assert.Equal(t, "10s", v.Data[0].Attributes.Arguments)
		assert.Equal(t, "print", v.Data[0].Attributes.WorkerType)
	}

	req5, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers?Worker=nojobforme", nil)
	assert.NoError(t, err)
	req5.Header.Add("Authorization", "Bearer "+tokenTriggers)
	res5, err := http.DefaultClient.Do(req5)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, http.StatusOK, res5.StatusCode)

	err = json.NewDecoder(res5.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}
	assert.Len(t, v.Data, 0)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "jobs_test")

	job.AddWorker(&job.WorkerConfig{
		WorkerType:  "print",
		Concurrency: 4,
		WorkerFunc: func(ctx *job.WorkerContext) error {
			var msg string
			if err := ctx.UnmarshalMessage(&msg); err != nil {
				return err
			}
			_, err := fmt.Println(msg)
			return err
		},
	})

	testInstance = setup.GetTestInstance()

	scope := strings.Join([]string{
		consts.Jobs + ":ALL:print:worker",
		consts.Triggers + ":ALL:print:worker",
	}, " ")
	token, _ = testInstance.MakeJWT(consts.CLIAudience, "CLI", scope,
		"", time.Now())

	ts = setup.GetTestServer("/jobs", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	os.Exit(setup.Run())
}
