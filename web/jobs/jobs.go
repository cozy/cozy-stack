package jobs

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/pkg/accounts"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/scheduler"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/echo"
	multierror "github.com/hashicorp/go-multierror"

	// konnectors is needed for bad triggers cleanup
	konnectors "github.com/cozy/cozy-stack/pkg/workers/konnectors"

	// import workers
	_ "github.com/cozy/cozy-stack/pkg/workers/log"
	_ "github.com/cozy/cozy-stack/pkg/workers/mails"
	_ "github.com/cozy/cozy-stack/pkg/workers/sharings"
	_ "github.com/cozy/cozy-stack/pkg/workers/thumbnail"
	_ "github.com/cozy/cozy-stack/pkg/workers/unzip"
)

type (
	apiJob struct {
		j *jobs.JobInfos
	}
	apiJobRequest struct {
		Arguments json.RawMessage  `json:"arguments"`
		Options   *jobs.JobOptions `json:"options"`
	}
	apiQueue struct {
		Count      int `json:"count"`
		workerType string
	}
	apiTrigger struct {
		t scheduler.Trigger
	}
	apiTriggerRequest struct {
		Type            string           `json:"type"`
		Arguments       string           `json:"arguments"`
		WorkerType      string           `json:"worker"`
		WorkerArguments json.RawMessage  `json:"worker_arguments"`
		Options         *jobs.JobOptions `json:"options"`
	}
)

func (j *apiJob) ID() string                             { return j.j.ID() }
func (j *apiJob) Rev() string                            { return j.j.Rev() }
func (j *apiJob) DocType() string                        { return consts.Jobs }
func (j *apiJob) Clone() couchdb.Doc                     { return j }
func (j *apiJob) SetID(_ string)                         {}
func (j *apiJob) SetRev(_ string)                        {}
func (j *apiJob) Relationships() jsonapi.RelationshipMap { return nil }
func (j *apiJob) Included() []jsonapi.Object             { return nil }
func (j *apiJob) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/jobs/" + j.j.WorkerType + "/" + j.j.ID()}
}
func (j *apiJob) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.j)
}

func (q *apiQueue) ID() string                             { return q.workerType }
func (q *apiQueue) Rev() string                            { return "" }
func (q *apiQueue) DocType() string                        { return consts.Queues }
func (q *apiQueue) Clone() couchdb.Doc                     { return q }
func (q *apiQueue) SetID(_ string)                         {}
func (q *apiQueue) SetRev(_ string)                        {}
func (q *apiQueue) Relationships() jsonapi.RelationshipMap { return nil }
func (q *apiQueue) Included() []jsonapi.Object             { return nil }
func (q *apiQueue) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/jobs/queue/" + q.workerType}
}
func (q *apiQueue) Valid(key, value string) bool {
	switch key {
	case "worker":
		return q.workerType == value
	}
	return false
}

func (t *apiTrigger) ID() string                             { return t.t.Infos().TID }
func (t *apiTrigger) Rev() string                            { return "" }
func (t *apiTrigger) DocType() string                        { return consts.Triggers }
func (t *apiTrigger) Clone() couchdb.Doc                     { return t }
func (t *apiTrigger) SetID(_ string)                         {}
func (t *apiTrigger) SetRev(_ string)                        {}
func (t *apiTrigger) Relationships() jsonapi.RelationshipMap { return nil }
func (t *apiTrigger) Included() []jsonapi.Object             { return nil }
func (t *apiTrigger) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/jobs/triggers/" + t.ID()}
}
func (t *apiTrigger) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.t.Infos())
}

func getQueue(c echo.Context) error {
	workerType := c.Param("worker-type")
	count, err := stack.GetBroker().QueueLen(workerType)
	if err != nil {
		return wrapJobsError(err)
	}
	o := &apiQueue{
		workerType: workerType,
		Count:      count,
	}

	if err := permissions.Allow(c, permissions.GET, o); err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, o, nil)
}

func pushJob(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	req := &apiJobRequest{}
	if _, err := jsonapi.Bind(c.Request(), &req); err != nil {
		return wrapJobsError(err)
	}

	jr := &jobs.JobRequest{
		Domain:     instance.Domain,
		WorkerType: c.Param("worker-type"),
		Options:    req.Options,
		Message: &jobs.Message{
			Type: jobs.JSONEncoding,
			Data: req.Arguments,
		},
	}
	if err := permissions.Allow(c, permissions.POST, jr); err != nil {
		return err
	}

	job, err := stack.GetBroker().PushJob(jr)
	if err != nil {
		return wrapJobsError(err)
	}

	return jsonapi.Data(c, http.StatusAccepted, &apiJob{job}, nil)
}

func newTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := stack.GetScheduler()
	req := &apiTriggerRequest{}
	if _, err := jsonapi.Bind(c.Request(), &req); err != nil {
		return wrapJobsError(err)
	}

	t, err := scheduler.NewTrigger(&scheduler.TriggerInfos{
		Type:       req.Type,
		WorkerType: req.WorkerType,
		Domain:     instance.Domain,
		Arguments:  req.Arguments,
		Options:    req.Options,
		Message: &jobs.Message{
			Type: jobs.JSONEncoding,
			Data: req.WorkerArguments,
		},
	})
	if err != nil {
		return wrapJobsError(err)
	}

	if err = permissions.Allow(c, permissions.POST, t); err != nil {
		return err
	}

	if err = sched.Add(t); err != nil {
		return wrapJobsError(err)
	}
	return jsonapi.Data(c, http.StatusCreated, &apiTrigger{t}, nil)
}

func getTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := stack.GetScheduler()
	t, err := sched.Get(instance.Domain, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err := permissions.Allow(c, permissions.GET, t); err != nil {
		return err
	}
	return jsonapi.Data(c, http.StatusOK, &apiTrigger{t}, nil)
}

func deleteTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := stack.GetScheduler()
	t, err := sched.Get(instance.Domain, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err := permissions.Allow(c, permissions.DELETE, t); err != nil {
		return err
	}
	if err := sched.Delete(instance.Domain, c.Param("trigger-id")); err != nil {
		return wrapJobsError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func cleanTriggers(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := stack.GetScheduler()
	if err := permissions.AllowWholeType(c, permissions.GET, consts.Triggers); err != nil {
		return err
	}
	if rsched, ok := sched.(*scheduler.RedisScheduler); ok {
		if err := rsched.ImportFromDB(instance.Domain); err != nil {
			return wrapJobsError(err)
		}
	}
	ts, err := sched.GetAll(instance.Domain)
	if err != nil {
		return wrapJobsError(err)
	}
	deleted := 0
	for _, t := range ts {
		infos := t.Infos()
		if infos.WorkerType == "konnector" {
			var msg konnectors.Options
			if err = infos.Message.Unmarshal(&msg); err != nil {
				if err = sched.Delete(instance.Domain, t.ID()); err != nil {
					logger.WithDomain(instance.Domain).Errorln("failed to delete orphan trigger", err)
				}
				deleted++
				continue
			}

			var a accounts.Account
			err = couchdb.GetDoc(instance, consts.Accounts, msg.Account, &a)
			if couchdb.IsNotFoundError(err) {
				if err = sched.Delete(instance.Domain, t.ID()); err != nil {
					logger.WithDomain(instance.Domain).Errorln("failed to delete orphan trigger", err)
				}
				deleted++
			}
		}
	}

	return c.JSON(200, map[string]int{"deleted": deleted})
}

func getAllTriggers(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	workerFilter := c.QueryParam("Worker")
	sched := stack.GetScheduler()
	if err := permissions.AllowWholeType(c, permissions.GET, consts.Triggers); err != nil {
		return err
	}
	ts, err := sched.GetAll(instance.Domain)
	if err != nil {
		return wrapJobsError(err)
	}
	objs := make([]jsonapi.Object, 0, len(ts))
	for _, t := range ts {
		if workerFilter == "" || t.Infos().WorkerType == workerFilter {
			objs = append(objs, &apiTrigger{t})
		}
	}
	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

func getJob(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	job, err := stack.GetBroker().GetJobInfos(instance.Domain, c.Param("job-id"))
	if err != nil {
		return err
	}
	if err := permissions.Allow(c, permissions.GET, job); err != nil {
		return err
	}
	return jsonapi.Data(c, http.StatusOK, &apiJob{job}, nil)
}

func cleanJobs(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var ups []*jobs.JobInfos
	now := time.Now()
	err := couchdb.ForeachDocs(instance, consts.Jobs, func(data []byte) error {
		var job *jobs.JobInfos
		if err := json.Unmarshal(data, &job); err != nil {
			return err
		}
		if job.State != jobs.Running {
			return nil
		}
		if job.StartedAt.Add(1 * time.Hour).Before(now) {
			ups = append(ups, job)
		}
		return nil
	})
	if err != nil && !couchdb.IsNoDatabaseError(err) {
		return err
	}
	var errf error
	for _, j := range ups {
		j.State = jobs.Done
		err := couchdb.UpdateDoc(instance, j)
		if err != nil {
			errf = multierror.Append(errf, err)
		}
	}
	if errf != nil {
		return errf
	}
	return c.JSON(200, map[string]int{"deleted": len(ups)})
}

// Routes sets the routing for the jobs service
func Routes(router *echo.Group) {
	router.GET("/queue/:worker-type", getQueue)
	router.POST("/queue/:worker-type", pushJob)

	router.GET("/triggers", getAllTriggers)
	router.POST("/triggers", newTrigger)
	router.POST("/triggers/clean", cleanTriggers)
	router.GET("/triggers/:trigger-id", getTrigger)
	router.DELETE("/triggers/:trigger-id", deleteTrigger)

	router.POST("/clean", cleanJobs)
	router.GET("/:job-id", getJob)
}

func wrapJobsError(err error) error {
	switch err {
	case scheduler.ErrNotFoundTrigger,
		jobs.ErrNotFoundJob,
		jobs.ErrUnknownWorker:
		return jsonapi.NotFound(err)
	case scheduler.ErrUnknownTrigger:
		return jsonapi.InvalidAttribute("Type", err)
	}
	return err
}
