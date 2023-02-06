package jobs

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	ts           *httptest.Server
	testInstance *instance.Instance
	token        string
)

type jobRequest struct {
	Arguments interface{} `json:"arguments"`
	Manual    bool        `json:"manual,omitempty"`
}

type jsonapiReq struct {
	Data *jsonapiData `json:"data"`
}

type jsonapiData struct {
	Attributes interface{} `json:"attributes"`
}

func TestJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())

	job.AddWorker(&job.WorkerConfig{
		WorkerType:  "print",
		Concurrency: 4,
		WorkerFunc: func(ctx *job.WorkerContext) error {
			var msg string
			if err := ctx.UnmarshalMessage(&msg); err != nil {
				return err
			}

			t.Log(msg)

			return nil
		},
	})

	testInstance = setup.GetTestInstance()

	scope := strings.Join([]string{
		consts.Jobs + ":ALL:print:worker",
		consts.Triggers + ":ALL:print:worker",
	}, " ")
	token, _ = testInstance.MakeJWT(consts.CLIAudience, "CLI", scope,
		"", time.Now())

	ts = setup.GetTestServer("/jobs", Routes, func(r *echo.Echo) *echo.Echo {
		r.Use(SetToken)
		return r
	})
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	t.Run("GetQueue", func(t *testing.T) {
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
	})

	t.Run("CreateJob", func(t *testing.T) {
		body, _ := json.Marshal(&jsonapiReq{
			Data: &jsonapiData{
				Attributes: &jobRequest{Arguments: "foobar"},
			},
		})
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/queue/print", bytes.NewReader(body))
		req.Header.Add("Authorization", "Bearer "+token)
		assert.NoError(t, err)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, 202, res.StatusCode)
		defer res.Body.Close()
		var resbody map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resbody)
		require.NoError(t, err)
		data, _ := resbody["data"].(map[string]interface{})
		attrs, _ := data["attributes"].(map[string]interface{})
		assert.Equal(t, "print", attrs["worker"])
		assert.NotEqual(t, true, attrs["manual_execution"])
	})

	t.Run("CreateManualJob", func(t *testing.T) {
		body, _ := json.Marshal(&jsonapiReq{
			Data: &jsonapiData{
				Attributes: &jobRequest{
					Arguments: "foobar",
					Manual:    true,
				},
			},
		})
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/queue/print", bytes.NewReader(body))
		req.Header.Add("Authorization", "Bearer "+token)
		assert.NoError(t, err)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, 202, res.StatusCode)
		var resbody map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&resbody)
		require.NoError(t, err)
		data, _ := resbody["data"].(map[string]interface{})
		attrs, _ := data["attributes"].(map[string]interface{})
		assert.Equal(t, "print", attrs["worker"])
		assert.Equal(t, true, attrs["manual_execution"])
	})

	t.Run("CreateJobForReservedWorker", func(t *testing.T) {
		body, _ := json.Marshal(&jsonapiReq{
			Data: &jsonapiData{
				Attributes: &jobRequest{Arguments: "foobar"},
			},
		})
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/queue/trash-files", bytes.NewReader(body))
		req.Header.Add("Authorization", "Bearer "+token)
		assert.NoError(t, err)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		assert.Equal(t, 403, res.StatusCode)
	})

	t.Run("CreateJobNotExist", func(t *testing.T) {
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
		require.NoError(t, err)

		assert.Equal(t, 404, res.StatusCode)
	})

	t.Run("AddGetAndDeleteTriggerAt", func(t *testing.T) {
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
		require.NoError(t, err)

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
		require.NoError(t, err)

		require.NotNil(t, v.Data)
		require.NotNil(t, v.Data.Attributes)
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
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, res2.StatusCode)

		req3, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req3.Header.Add("Authorization", "Bearer "+token)
		res3, err := http.DefaultClient.Do(req3)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res3.StatusCode)

		req4, err := http.NewRequest("DELETE", ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req4.Header.Add("Authorization", "Bearer "+token)
		res4, err := http.DefaultClient.Do(req4)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNoContent, res4.StatusCode)

		req5, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req5.Header.Add("Authorization", "Bearer "+token)
		res5, err := http.DefaultClient.Do(req5)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNotFound, res5.StatusCode)
	})

	t.Run("AddGetAndDeleteTriggerIn", func(t *testing.T) {
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
		require.NoError(t, err)

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
		require.NoError(t, err)

		require.NotNil(t, v.Data)
		require.NotNil(t, v.Data.Attributes)
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
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, res2.StatusCode)

		req3, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req3.Header.Add("Authorization", "Bearer "+token)
		res3, err := http.DefaultClient.Do(req3)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res3.StatusCode)

		req4, err := http.NewRequest("DELETE", ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req4.Header.Add("Authorization", "Bearer "+token)
		res4, err := http.DefaultClient.Do(req4)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNoContent, res4.StatusCode)

		req5, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req5.Header.Add("Authorization", "Bearer "+token)
		res5, err := http.DefaultClient.Do(req5)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNotFound, res5.StatusCode)
	})

	t.Run("AddGetUpdateAndDeleteTriggerCron", func(t *testing.T) {
		body, _ := json.Marshal(&jsonapiReq{
			Data: &jsonapiData{
				Attributes: &map[string]interface{}{
					"type":      "@cron",
					"arguments": "0 0 0 * * 0",
					"worker":    "print",
					"message":   "foo",
				},
			},
		})
		req1, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/triggers", bytes.NewReader(body))
		assert.NoError(t, err)
		req1.Header.Add("Authorization", "Bearer "+token)
		res1, err := http.DefaultClient.Do(req1)
		require.NoError(t, err)

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
		require.NoError(t, err)

		require.NotNil(t, v.Data)
		require.NotNil(t, v.Data.Attributes)
		triggerID := v.Data.ID
		assert.Equal(t, consts.Triggers, v.Data.Type)
		assert.Equal(t, "@cron", v.Data.Attributes.Type)
		assert.Equal(t, "0 0 0 * * 0", v.Data.Attributes.Arguments)
		assert.Equal(t, "print", v.Data.Attributes.WorkerType)

		body, _ = json.Marshal(&jsonapiReq{
			Data: &jsonapiData{
				Attributes: map[string]interface{}{
					"arguments": "0 0 0 * * 1",
				},
			},
		})
		req2, err := http.NewRequest(http.MethodPatch, ts.URL+"/jobs/triggers/"+triggerID, bytes.NewReader(body))
		assert.NoError(t, err)
		req2.Header.Add("Authorization", "Bearer "+token)
		res2, err := http.DefaultClient.Do(req2)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res2.StatusCode)

		req3, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req3.Header.Add("Authorization", "Bearer "+token)
		res3, err := http.DefaultClient.Do(req3)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res3.StatusCode)

		var v2 struct {
			Data struct {
				ID         string            `json:"id"`
				Type       string            `json:"type"`
				Attributes *job.TriggerInfos `json:"attributes"`
			}
		}
		err = json.NewDecoder(res3.Body).Decode(&v2)
		require.NoError(t, err)

		require.NotNil(t, v2.Data)
		require.NotNil(t, v2.Data.Attributes)
		assert.Equal(t, triggerID, v2.Data.ID)
		assert.Equal(t, consts.Triggers, v2.Data.Type)
		assert.Equal(t, "@cron", v2.Data.Attributes.Type)
		assert.Equal(t, "0 0 0 * * 1", v2.Data.Attributes.Arguments)
		assert.Equal(t, "print", v2.Data.Attributes.WorkerType)

		req4, err := http.NewRequest("DELETE", ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req4.Header.Add("Authorization", "Bearer "+token)
		res4, err := http.DefaultClient.Do(req4)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNoContent, res4.StatusCode)
	})

	t.Run("AddTriggerWithMetadata", func(t *testing.T) {
		at := time.Now().Add(1100 * time.Millisecond).Format(time.RFC3339)
		body, _ := json.Marshal(&jsonapiReq{
			Data: &jsonapiData{
				Attributes: map[string]interface{}{
					"type":      "@webhook",
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
		require.NoError(t, err)

		defer res1.Body.Close()
		assert.Equal(t, http.StatusCreated, res1.StatusCode)

		var v struct {
			Data struct {
				ID         string            `json:"id"`
				Type       string            `json:"type"`
				Attributes *job.TriggerInfos `json:"attributes"`
				Links      jsonapi.LinksList `json:"links"`
			}
		}

		err = json.NewDecoder(res1.Body).Decode(&v)
		require.NoError(t, err)

		require.NotNil(t, v.Data)
		require.NotNil(t, v.Data.Attributes)
		triggerID := v.Data.ID
		assert.Equal(t, consts.Triggers, v.Data.Type)
		assert.Equal(t, "@webhook", v.Data.Attributes.Type)
		assert.Equal(t, "https://"+testInstance.Domain+"/jobs/webhooks/"+triggerID, v.Data.Links.Webhook)
		assert.Equal(t, at, v.Data.Attributes.Arguments)
		assert.Equal(t, "print", v.Data.Attributes.WorkerType)

		assert.Equal(t, "1", v.Data.Attributes.Metadata.DocTypeVersion)
		assert.Equal(t, 1, v.Data.Attributes.Metadata.MetadataVersion)
		// "CLI" is the token subject
		assert.Equal(t, "CLI", v.Data.Attributes.Metadata.CreatedByApp)
		assert.NotZero(t, v.Data.Attributes.Metadata.CreatedAt)
		assert.NotZero(t, v.Data.Attributes.Metadata.UpdatedAt)

		req2, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req2.Header.Add("Authorization", "Bearer "+token)
		res2, err := http.DefaultClient.Do(req2)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res2.StatusCode)

		// Clean
		req3, err := http.NewRequest("DELETE", ts.URL+"/jobs/triggers/"+triggerID, nil)
		assert.NoError(t, err)
		req3.Header.Add("Authorization", "Bearer "+token)
		res3, err := http.DefaultClient.Do(req3)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNoContent, res3.StatusCode)
	})

	t.Run("GetAllJobs", func(t *testing.T) {
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
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res1.StatusCode)

		err = json.NewDecoder(res1.Body).Decode(&v)
		require.NoError(t, err)

		assert.Len(t, v.Data, 0)

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
		require.NoError(t, err)

		assert.Equal(t, http.StatusCreated, res2.StatusCode)

		req3, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers", nil)
		assert.NoError(t, err)
		req3.Header.Add("Authorization", "Bearer "+tokenTriggers)
		res3, err := http.DefaultClient.Do(req3)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res3.StatusCode)

		err = json.NewDecoder(res3.Body).Decode(&v)
		require.NoError(t, err)

		if assert.Len(t, v.Data, 1) {
			assert.Equal(t, consts.Triggers, v.Data[0].Type)
			assert.Equal(t, "@in", v.Data[0].Attributes.Type)
			assert.Equal(t, "10s", v.Data[0].Attributes.Arguments)
			assert.Equal(t, "print", v.Data[0].Attributes.WorkerType)
		}

		req4, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers?Worker=print", nil)
		assert.NoError(t, err)
		req4.Header.Add("Authorization", "Bearer "+tokenTriggers)
		res4, err := http.DefaultClient.Do(req4)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res4.StatusCode)

		err = json.NewDecoder(res4.Body).Decode(&v)
		require.NoError(t, err)

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
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res5.StatusCode)

		err = json.NewDecoder(res5.Body).Decode(&v)
		require.NoError(t, err)

		assert.Len(t, v.Data, 0)

		req6, err := http.NewRequest(http.MethodGet, ts.URL+"/jobs/triggers?Type=@in", nil)
		assert.NoError(t, err)
		req6.Header.Add("Authorization", "Bearer "+tokenTriggers)
		res6, err := http.DefaultClient.Do(req6)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res6.StatusCode)

		err = json.NewDecoder(res6.Body).Decode(&v)
		require.NoError(t, err)

		if assert.Len(t, v.Data, 1) {
			assert.Equal(t, consts.Triggers, v.Data[0].Type)
			assert.Equal(t, "@in", v.Data[0].Attributes.Type)
			assert.Equal(t, "10s", v.Data[0].Attributes.Arguments)
			assert.Equal(t, "print", v.Data[0].Attributes.WorkerType)
		}
	})

	t.Run("ClientJobs", func(t *testing.T) {
		scope := consts.Jobs + " " + consts.Triggers
		token2, _ := testInstance.MakeJWT(consts.CLIAudience, "CLI", scope, "", time.Now())

		body, _ := json.Marshal(&jsonapiReq{
			Data: &jsonapiData{
				Attributes: &map[string]interface{}{
					"type":    "@client",
					"message": "foobar",
				},
			},
		})
		req1, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/triggers", bytes.NewReader(body))
		assert.NoError(t, err)
		req1.Header.Add("Authorization", "Bearer "+token2)
		res1, err := http.DefaultClient.Do(req1)
		require.NoError(t, err)

		defer res1.Body.Close()
		assert.Equal(t, http.StatusCreated, res1.StatusCode)

		var v1 struct {
			Data struct {
				ID         string            `json:"id"`
				Type       string            `json:"type"`
				Attributes *job.TriggerInfos `json:"attributes"`
			}
		}
		err = json.NewDecoder(res1.Body).Decode(&v1)
		require.NoError(t, err)

		require.NotNil(t, v1.Data)
		require.NotNil(t, v1.Data.Attributes)
		triggerID := v1.Data.ID
		assert.Equal(t, consts.Triggers, v1.Data.Type)
		assert.Equal(t, "@client", v1.Data.Attributes.Type)
		assert.Equal(t, "client", v1.Data.Attributes.WorkerType)

		req2, err := http.NewRequest(http.MethodPost, ts.URL+"/jobs/triggers/"+triggerID+"/launch", nil)
		assert.NoError(t, err)
		req2.Header.Add("Authorization", "Bearer "+token2)
		res2, err := http.DefaultClient.Do(req2)
		require.NoError(t, err)

		defer res2.Body.Close()
		assert.Equal(t, http.StatusCreated, res2.StatusCode)

		var v2 struct {
			Data struct {
				ID         string   `json:"id"`
				Type       string   `json:"type"`
				Attributes *job.Job `json:"attributes"`
			}
		}
		err = json.NewDecoder(res2.Body).Decode(&v2)
		require.NoError(t, err)

		require.NotNil(t, v2.Data)
		require.NotNil(t, v2.Data.Attributes)
		jobID := v2.Data.ID
		assert.Equal(t, consts.Jobs, v2.Data.Type)
		assert.Equal(t, "client", v2.Data.Attributes.WorkerType)
		assert.Equal(t, job.Running, v2.Data.Attributes.State)
		assert.NotEmpty(t, v2.Data.Attributes.QueuedAt)
		assert.NotEmpty(t, v2.Data.Attributes.StartedAt)

		body3, _ := json.Marshal(&jsonapiReq{
			Data: &jsonapiData{
				Attributes: &map[string]interface{}{
					"state": "errored",
					"error": "LOGIN_FAILED",
				},
			},
		})
		req3, err := http.NewRequest(http.MethodPatch, ts.URL+"/jobs/"+jobID, bytes.NewReader(body3))
		assert.NoError(t, err)
		req3.Header.Add("Authorization", "Bearer "+token2)
		res3, err := http.DefaultClient.Do(req3)
		require.NoError(t, err)

		defer res3.Body.Close()
		assert.Equal(t, http.StatusOK, res3.StatusCode)

		var v3 struct {
			Data struct {
				ID         string   `json:"id"`
				Type       string   `json:"type"`
				Attributes *job.Job `json:"attributes"`
			}
		}
		err = json.NewDecoder(res3.Body).Decode(&v3)
		require.NoError(t, err)

		require.NotNil(t, v3.Data)
		require.NotNil(t, v3.Data.Attributes)
		assert.Equal(t, consts.Jobs, v3.Data.Type)
		assert.Equal(t, "client", v3.Data.Attributes.WorkerType)
		assert.Equal(t, job.Errored, v3.Data.Attributes.State)
		assert.Equal(t, "LOGIN_FAILED", v3.Data.Attributes.Error)
		assert.NotEmpty(t, v3.Data.Attributes.QueuedAt)
		assert.NotEmpty(t, v3.Data.Attributes.StartedAt)
		assert.NotEmpty(t, v3.Data.Attributes.FinishedAt)
	})
}

func SetToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		tok := middlewares.GetRequestToken(c)
		// Forcing the token parsing to have the "claims" parameter in the
		// context (in production, it is done via
		// middlewares.CheckInstanceBlocked)
		_, err := middlewares.ParseJWT(c, testInstance, tok)
		if err != nil {
			return err
		}
		return next(c)
	}
}
