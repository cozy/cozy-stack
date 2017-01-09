package jobs

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

const TypeTextEventStream = "text/event-stream"

type apiJob struct {
	j *jobs.JobInfos
}

func (j *apiJob) ID() string      { return j.j.ID }
func (j *apiJob) Rev() string     { return "" }
func (j *apiJob) DocType() string { return consts.Jobs }
func (j *apiJob) SetID(_ string)  {}
func (j *apiJob) SetRev(_ string) {}
func (j *apiJob) SelfLink() string {
	return "/jobs/" + j.j.WorkerType + "/" + j.j.ID
}
func (j *apiJob) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}
func (j *apiJob) Included() []jsonapi.Object {
	return nil
}
func (j *apiJob) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.j)
}

type apiJobRequest struct {
	Arguments json.RawMessage
	Options   *jobs.JobOptions
}

func pushJob(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	req := &apiJobRequest{}
	if err := c.Bind(&req); err != nil {
		return err
	}

	job, ch, err := instance.JobsBroker().PushJob(&jobs.JobRequest{
		WorkerType: c.Param("worker-type"),
		Options:    req.Options,
		Message: &jobs.Message{
			Type: jobs.JSONEncoding,
			Data: req.Arguments,
		},
	})
	if err != nil {
		return wrapJobsError(err)
	}

	accept := c.Request().Header.Get("Accept")
	if accept != TypeTextEventStream {
		return jsonapi.Data(c, http.StatusAccepted, &apiJob{job}, nil)
	}

	w := c.Response().Writer
	w.Header().Set("Content-Type", TypeTextEventStream)
	w.WriteHeader(200)
	if err := sendJob(job, w); err != nil {
		return nil
	}
	for job = range ch {
		if err := sendJob(job, w); err != nil {
			return nil
		}
	}
	return nil
}

func sendJob(job *jobs.JobInfos, w http.ResponseWriter) error {
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	s := fmt.Sprintf("event: %s\r\ndata: %s\r\n\r\n", job.State, b)
	_, err = w.Write([]byte(s))
	if err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// Routes sets the routing for the jobs service
func Routes(router *echo.Group) {
	router.POST("/queue/:worker-type", pushJob)
}

func wrapJobsError(err error) error {
	switch err {
	case jobs.ErrUnknownWorker:
		return jsonapi.NotFound(err)
	}
	return err
}
