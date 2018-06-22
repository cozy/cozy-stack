package jobs

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/echo"
	multierror "github.com/hashicorp/go-multierror"

	// import workers
	_ "github.com/cozy/cozy-stack/pkg/workers/exec"
	_ "github.com/cozy/cozy-stack/pkg/workers/log"
	_ "github.com/cozy/cozy-stack/pkg/workers/mails"
	_ "github.com/cozy/cozy-stack/pkg/workers/migrations"
	_ "github.com/cozy/cozy-stack/pkg/workers/move"
	_ "github.com/cozy/cozy-stack/pkg/workers/push"
	_ "github.com/cozy/cozy-stack/pkg/workers/share"
	_ "github.com/cozy/cozy-stack/pkg/workers/thumbnail"
	_ "github.com/cozy/cozy-stack/pkg/workers/unzip"
	_ "github.com/cozy/cozy-stack/pkg/workers/updates"
)

type (
	apiJob struct {
		j *jobs.Job
	}
	apiJobRequest struct {
		Arguments json.RawMessage  `json:"arguments"`
		Options   *jobs.JobOptions `json:"options"`
	}
	apiQueue struct {
		workerType string
	}
	apiTrigger struct {
		t *jobs.TriggerInfos
	}
	apiTriggerState struct {
		t *jobs.TriggerInfos
		s *jobs.TriggerState
	}
	apiTriggerRequest struct {
		Type            string           `json:"type"`
		Arguments       string           `json:"arguments"`
		WorkerType      string           `json:"worker"`
		WorkerArguments json.RawMessage  `json:"worker_arguments"`
		Debounce        string           `json:"debounce"`
		Options         *jobs.JobOptions `json:"options"`
	}
)

func (j apiJob) ID() string                             { return j.j.ID() }
func (j apiJob) Rev() string                            { return j.j.Rev() }
func (j apiJob) DocType() string                        { return consts.Jobs }
func (j apiJob) Clone() couchdb.Doc                     { return j }
func (j apiJob) SetID(_ string)                         {}
func (j apiJob) SetRev(_ string)                        {}
func (j apiJob) Relationships() jsonapi.RelationshipMap { return nil }
func (j apiJob) Included() []jsonapi.Object             { return nil }
func (j apiJob) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/jobs/" + j.j.WorkerType + "/" + j.j.ID()}
}
func (j apiJob) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.j)
}

func (q apiQueue) ID() string      { return q.workerType }
func (q apiQueue) DocType() string { return consts.Jobs }
func (q apiQueue) Match(key, value string) bool {
	switch key {
	case "worker":
		return q.workerType == value
	}
	return false
}

func (t apiTrigger) ID() string                             { return t.t.TID }
func (t apiTrigger) Rev() string                            { return "" }
func (t apiTrigger) DocType() string                        { return consts.Triggers }
func (t apiTrigger) Clone() couchdb.Doc                     { return t }
func (t apiTrigger) SetID(_ string)                         {}
func (t apiTrigger) SetRev(_ string)                        {}
func (t apiTrigger) Relationships() jsonapi.RelationshipMap { return nil }
func (t apiTrigger) Included() []jsonapi.Object             { return nil }
func (t apiTrigger) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/jobs/triggers/" + t.ID()}
}
func (t apiTrigger) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.t)
}

func (t apiTriggerState) ID() string                             { return t.t.TID }
func (t apiTriggerState) Rev() string                            { return "" }
func (t apiTriggerState) DocType() string                        { return consts.TriggersState }
func (t apiTriggerState) Clone() couchdb.Doc                     { return t }
func (t apiTriggerState) SetID(_ string)                         {}
func (t apiTriggerState) SetRev(_ string)                        {}
func (t apiTriggerState) Relationships() jsonapi.RelationshipMap { return nil }
func (t apiTriggerState) Included() []jsonapi.Object             { return nil }
func (t apiTriggerState) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/jobs/triggers/" + t.ID() + "/state"}
}
func (t apiTriggerState) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.s)
}

func getQueue(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	workerType := c.Param("worker-type")

	o := apiQueue{workerType: workerType}
	// TODO: uncomment to restric jobs permissions.
	// if err := permissions.AllowOnFields(c, permissions.GET, o, "worker"); err != nil {
	// 	return err
	// }
	if err := permissions.Allow(c, permissions.GET, o); err != nil {
		return err
	}

	js, err := jobs.GetQueuedJobs(instance, workerType)
	if err != nil {
		return wrapJobsError(err)
	}

	objs := make([]jsonapi.Object, len(js))
	for i, j := range js {
		objs[i] = apiJob{j}
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

func pushJob(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	req := apiJobRequest{}
	if _, err := jsonapi.Bind(c.Request().Body, &req); err != nil {
		return wrapJobsError(err)
	}

	jr := &jobs.JobRequest{
		WorkerType: c.Param("worker-type"),
		Options:    req.Options,
		Message:    jobs.Message(req.Arguments),
	}
	// TODO: uncomment to restric jobs permissions.
	// if err := permissions.AllowOnFields(c, permissions.POST, jr, "worker"); err != nil {
	// 	return err
	// }
	if err := permissions.Allow(c, permissions.POST, jr); err != nil {
		return err
	}

	job, err := jobs.System().PushJob(instance, jr)
	if err != nil {
		return wrapJobsError(err)
	}

	return jsonapi.Data(c, http.StatusAccepted, apiJob{job}, nil)
}

func newTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := jobs.System()
	req := apiTriggerRequest{}
	if _, err := jsonapi.Bind(c.Request().Body, &req); err != nil {
		return wrapJobsError(err)
	}

	if req.Debounce != "" {
		if _, err := time.ParseDuration(req.Debounce); err != nil {
			return jsonapi.InvalidAttribute("debounce", err)
		}
	}

	t, err := jobs.NewTrigger(instance, jobs.TriggerInfos{
		Type:       req.Type,
		WorkerType: req.WorkerType,
		Domain:     instance.Domain,
		Arguments:  req.Arguments,
		Debounce:   req.Debounce,
		Options:    req.Options,
	}, req.WorkerArguments)
	if err != nil {
		return wrapJobsError(err)
	}
	// TODO: uncomment to restric jobs permissions.
	// if err = permissions.AllowOnFields(c, permissions.POST, t, "worker"); err != nil {
	// 	return err
	// }
	if err = permissions.Allow(c, permissions.POST, t); err != nil {
		return err
	}

	if err = sched.AddTrigger(t); err != nil {
		return wrapJobsError(err)
	}
	return jsonapi.Data(c, http.StatusCreated, apiTrigger{t.Infos()}, nil)
}

func getTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := jobs.System()
	t, err := sched.GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err = permissions.Allow(c, permissions.GET, t); err != nil {
		return err
	}
	tInfos := t.Infos()
	tInfos.CurrentState, err = jobs.GetTriggerState(t)
	if err != nil {
		return wrapJobsError(err)
	}
	return jsonapi.Data(c, http.StatusOK, apiTrigger{tInfos}, nil)
}

func getTriggerState(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := jobs.System()
	t, err := sched.GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err = permissions.Allow(c, permissions.GET, t); err != nil {
		return err
	}
	state, err := jobs.GetTriggerState(t)
	if err != nil {
		return wrapJobsError(err)
	}
	return jsonapi.Data(c, http.StatusOK, apiTriggerState{t: t.Infos(), s: state}, nil)
}

func getTriggerJobs(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	var err error

	var limit int
	if queryLimit := c.QueryParam("Limit"); queryLimit != "" {
		limit, err = strconv.Atoi(queryLimit)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}
	}

	sched := jobs.System()
	t, err := sched.GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err = permissions.Allow(c, permissions.GET, t); err != nil {
		return err
	}

	js, err := jobs.GetJobs(t, limit)
	if err != nil {
		return wrapJobsError(err)
	}

	objs := make([]jsonapi.Object, len(js))
	for i, j := range js {
		objs[i] = apiJob{j}
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

func launchTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	t, err := jobs.System().GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err = permissions.Allow(c, permissions.POST, t); err != nil {
		return err
	}
	req := t.Infos().JobRequest()
	req.Manual = true
	j, err := jobs.System().PushJob(instance, req)
	if err != nil {
		return wrapJobsError(err)
	}
	return jsonapi.Data(c, http.StatusCreated, apiJob{j}, nil)
}

func deleteTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := jobs.System()
	t, err := sched.GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err := permissions.Allow(c, permissions.DELETE, t); err != nil {
		return err
	}
	if err := sched.DeleteTrigger(instance, c.Param("trigger-id")); err != nil {
		return wrapJobsError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func getAllTriggers(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	workerType := c.QueryParam("Worker")

	if err := permissions.AllowWholeType(c, permissions.GET, consts.Triggers); err != nil {
		if workerType == "" {
			return err
		}
		o := &jobs.TriggerInfos{WorkerType: workerType}
		if err := permissions.AllowOnFields(c, permissions.GET, o, "worker"); err != nil {
			return err
		}
	}

	sched := jobs.System()
	ts, err := sched.GetAllTriggers(instance)
	if err != nil {
		return wrapJobsError(err)
	}

	// TODO: we could potentially benefit from an index on 'worker_type' field.
	objs := make([]jsonapi.Object, 0, len(ts))
	for _, t := range ts {
		tInfos := t.Infos()
		if workerType == "" || tInfos.WorkerType == workerType {
			tInfos.CurrentState, err = jobs.GetTriggerState(t)
			if err != nil {
				return wrapJobsError(err)
			}
			objs = append(objs, apiTrigger{tInfos})
		}
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

func getJob(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	job, err := jobs.Get(instance, c.Param("job-id"))
	if err != nil {
		return err
	}
	if err := permissions.Allow(c, permissions.GET, job); err != nil {
		return err
	}
	return jsonapi.Data(c, http.StatusOK, apiJob{job}, nil)
}

func cleanJobs(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if err := permissions.AllowWholeType(c, permissions.POST, consts.Jobs); err != nil {
		return err
	}
	var ups []*jobs.Job
	now := time.Now()
	err := couchdb.ForeachDocs(instance, consts.Jobs, func(_ string, data json.RawMessage) error {
		var job *jobs.Job
		if err := json.Unmarshal(data, &job); err != nil {
			return err
		}
		if job.State == jobs.Running || job.State == jobs.Queued {
			if job.StartedAt.Add(1 * time.Hour).Before(now) {
				ups = append(ups, job)
			}
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

	router.POST("/triggers", newTrigger)
	router.GET("/triggers", getAllTriggers)
	router.GET("/triggers/:trigger-id", getTrigger)
	router.GET("/triggers/:trigger-id/state", getTriggerState)
	router.GET("/triggers/:trigger-id/jobs", getTriggerJobs)
	router.POST("/triggers/:trigger-id/launch", launchTrigger)
	router.DELETE("/triggers/:trigger-id", deleteTrigger)

	router.POST("/clean", cleanJobs)
	router.GET("/:job-id", getJob)
}

func wrapJobsError(err error) error {
	switch err {
	case jobs.ErrNotFoundTrigger,
		jobs.ErrNotFoundJob,
		jobs.ErrUnknownWorker:
		return jsonapi.NotFound(err)
	case jobs.ErrUnknownTrigger:
		return jsonapi.InvalidAttribute("Type", err)
	}
	return err
}
