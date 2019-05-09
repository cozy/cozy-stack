package jobs

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/justincampbell/bigduration"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	multierror "github.com/hashicorp/go-multierror"

	// import workers
	"github.com/cozy/cozy-stack/worker/exec"
	_ "github.com/cozy/cozy-stack/worker/log"
	_ "github.com/cozy/cozy-stack/worker/mails"
	_ "github.com/cozy/cozy-stack/worker/migrations"
	_ "github.com/cozy/cozy-stack/worker/move"
	_ "github.com/cozy/cozy-stack/worker/push"
	_ "github.com/cozy/cozy-stack/worker/share"
	_ "github.com/cozy/cozy-stack/worker/thumbnail"
	_ "github.com/cozy/cozy-stack/worker/unzip"
	_ "github.com/cozy/cozy-stack/worker/updates"
)

type (
	apiJob struct {
		j *job.Job
	}
	apiJobRequest struct {
		Arguments   json.RawMessage `json:"arguments"`
		ForwardLogs bool            `json:"forward_logs"`
		Options     *job.JobOptions `json:"options"`
	}
	apiQueue struct {
		workerType string
	}
	apiTrigger struct {
		t *job.TriggerInfos
	}
	apiTriggerState struct {
		t *job.TriggerInfos
		s *job.TriggerState
	}
	apiTriggerRequest struct {
		Type            string          `json:"type"`
		Arguments       string          `json:"arguments"`
		WorkerType      string          `json:"worker"`
		Message         json.RawMessage `json:"message"`
		WorkerArguments json.RawMessage `json:"worker_arguments"`
		Debounce        string          `json:"debounce"`
		Options         *job.JobOptions `json:"options"`
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
	// if err := middlewares.AllowOnFields(c, permission.GET, o, "worker"); err != nil {
	// 	return err
	// }
	if err := middlewares.Allow(c, permission.GET, o); err != nil {
		return err
	}

	js, err := job.GetQueuedJobs(instance, workerType)
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

	jr := &job.JobRequest{
		WorkerType:  c.Param("worker-type"),
		Options:     req.Options,
		ForwardLogs: req.ForwardLogs,
		Message:     job.Message(req.Arguments),
	}

	// TODO: uncomment to restric jobs permissions.
	// if err := middlewares.AllowOnFields(c, permission.POST, jr, "worker"); err != nil {
	// 	return err
	// }
	if err := middlewares.Allow(c, permission.POST, jr); err != nil {
		return err
	}

	permd, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if jr.ForwardLogs && permd.Type != permission.TypeCLI {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	j, err := job.System().PushJob(instance, jr)
	if err != nil {
		return wrapJobsError(err)
	}

	return jsonapi.Data(c, http.StatusAccepted, apiJob{j}, nil)
}

func newTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := job.System()
	req := apiTriggerRequest{}
	if _, err := jsonapi.Bind(c.Request().Body, &req); err != nil {
		return wrapJobsError(err)
	}

	if req.Debounce != "" {
		if _, err := time.ParseDuration(req.Debounce); err != nil {
			return jsonapi.InvalidAttribute("debounce", err)
		}
	}

	msg := req.Message
	if req.Message == nil || len(req.Message) == 0 {
		msg = req.WorkerArguments
	}
	t, err := job.NewTrigger(instance, job.TriggerInfos{
		Type:       req.Type,
		WorkerType: req.WorkerType,
		Domain:     instance.Domain,
		Arguments:  req.Arguments,
		Debounce:   req.Debounce,
		Options:    req.Options,
	}, msg)
	if err != nil {
		return wrapJobsError(err)
	}
	// TODO: uncomment to restric jobs permissions.
	// if err = middlewares.AllowOnFields(c, permission.POST, t, "worker"); err != nil {
	// 	return err
	// }
	if err = middlewares.Allow(c, permission.POST, t); err != nil {
		return err
	}

	if err = sched.AddTrigger(t); err != nil {
		return wrapJobsError(err)
	}
	return jsonapi.Data(c, http.StatusCreated, apiTrigger{t.Infos()}, nil)
}

func getTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := job.System()
	t, err := sched.GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err = middlewares.Allow(c, permission.GET, t); err != nil {
		return err
	}
	tInfos := t.Infos()
	tInfos.CurrentState, err = job.GetTriggerState(t, t.ID())
	if err != nil {
		return wrapJobsError(err)
	}
	return jsonapi.Data(c, http.StatusOK, apiTrigger{tInfos}, nil)
}

func getTriggerState(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := job.System()
	t, err := sched.GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}

	if err = middlewares.Allow(c, permission.GET, t); err != nil {
		// For konnector trigger, we have a specific logic to check for
		// permissions: if the application permissions and the konnector
		// permissions intersect on one or more doctypes, we give the right to the
		// application to read the trigger state.
		if ok := extractKonnectorPermissions(c, instance, t); !ok {
			return middlewares.ErrForbidden
		}
	}

	state, err := job.GetTriggerState(t, t.ID())
	if err != nil {
		return wrapJobsError(err)
	}
	return jsonapi.Data(c, http.StatusOK, apiTriggerState{t: t.Infos(), s: state}, nil)
}

func extractKonnectorPermissions(c echo.Context, i *instance.Instance, t job.Trigger) (ok bool) {
	if t.Infos().WorkerType != "konnector" {
		return
	}
	reqDoc, err := middlewares.GetPermission(c)
	if err != nil {
		return
	}
	var msg exec.KonnectorMessage
	if err = t.Infos().Message.Unmarshal(&msg); err != nil {
		return
	}
	var man *app.KonnManifest
	if man, err = app.GetKonnectorBySlug(i, msg.Konnector); err != nil {
		return
	}
	reqRules := reqDoc.Permissions
	appRules := man.Permissions()
	return intersectPermissions(reqRules, appRules)
}

func intersectPermissions(rules1, rules2 permission.Set) bool {
	doctypeBlacklist := []string{consts.Settings}
	// This rule intersection only cross permissions on whole doctypes (no
	// values).
	wholeDoctypes := make(map[string]struct{})
	for _, rule := range rules1 {
		if len(rule.Values) == 0 && !utils.IsInArray(rule.Type, doctypeBlacklist) {
			wholeDoctypes[rule.Type] = struct{}{}
		}
	}
	for _, rule := range rules2 {
		if len(rule.Values) == 0 {
			if _, ok := wholeDoctypes[rule.Type]; ok {
				return true
			}
		}
	}
	return false
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

	sched := job.System()
	t, err := sched.GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err = middlewares.Allow(c, permission.GET, t); err != nil {
		return err
	}

	js, err := job.GetJobs(t, t.ID(), limit)
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
	t, err := job.System().GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err = middlewares.Allow(c, permission.POST, t); err != nil {
		return err
	}
	req := t.Infos().JobRequest()
	req.Manual = true
	j, err := job.System().PushJob(instance, req)
	if err != nil {
		return wrapJobsError(err)
	}
	return jsonapi.Data(c, http.StatusCreated, apiJob{j}, nil)
}

func deleteTrigger(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sched := job.System()
	t, err := sched.GetTrigger(instance, c.Param("trigger-id"))
	if err != nil {
		return wrapJobsError(err)
	}
	if err := middlewares.Allow(c, permission.DELETE, t); err != nil {
		// See getTriggerState: we have a specific permissions rule to allow an
		// application to read or delete a trigger
		if ok := extractKonnectorPermissions(c, instance, t); !ok {
			return middlewares.ErrForbidden
		}
	}
	if err := sched.DeleteTrigger(instance, c.Param("trigger-id")); err != nil {
		return wrapJobsError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func getAllTriggers(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	workerType := c.QueryParam("Worker")

	if err := middlewares.AllowWholeType(c, permission.GET, consts.Triggers); err != nil {
		if workerType == "" {
			return err
		}
		o := &job.TriggerInfos{WorkerType: workerType}
		if err := middlewares.AllowOnFields(c, permission.GET, o, "worker"); err != nil {
			return err
		}
	}

	sched := job.System()
	ts, err := sched.GetAllTriggers(instance)
	if err != nil {
		return wrapJobsError(err)
	}

	// TODO: we could potentially benefit from an index on 'worker_type' field.
	objs := make([]jsonapi.Object, 0, len(ts))
	for _, t := range ts {
		tInfos := t.Infos()
		if workerType == "" || tInfos.WorkerType == workerType {
			tInfos.CurrentState, err = job.GetTriggerState(t, t.ID())
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
	j, err := job.Get(instance, c.Param("job-id"))
	if err != nil {
		return err
	}
	if err := middlewares.Allow(c, permission.GET, j); err != nil {
		return err
	}
	return jsonapi.Data(c, http.StatusOK, apiJob{j}, nil)
}

func cleanJobs(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Jobs); err != nil {
		return err
	}
	var ups []*job.Job
	now := time.Now()
	err := couchdb.ForeachDocs(instance, consts.Jobs, func(_ string, data json.RawMessage) error {
		var j *job.Job
		if err := json.Unmarshal(data, &j); err != nil {
			return err
		}
		if j.State == job.Running || j.State == job.Queued {
			if j.StartedAt.Add(1 * time.Hour).Before(now) {
				ups = append(ups, j)
			}
		}
		return nil
	})
	if err != nil && !couchdb.IsNoDatabaseError(err) {
		return err
	}
	var errf error
	for _, j := range ups {
		j.State = job.Done
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

func purgeJobs(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.DELETE, consts.Jobs); err != nil {
		return err
	}

	workersParam := c.QueryParam("workers")
	durationParam := c.QueryParam("duration")

	conf := config.GetConfig().Jobs
	dur, err := bigduration.ParseDuration(conf.DefaultDurationToKeep)
	if err != nil {
		return err
	}

	if durationParam != "" {
		dur, err = bigduration.ParseDuration(durationParam)
		if err != nil {
			return err
		}
	}
	workers := job.GetWorkersNamesList()
	if workersParam != "" {
		workers = strings.Split(workersParam, ",")
	}

	// Step 1: We want to get all the jobs prior to the date parameter.
	// Jobs returned are the ones we want to remove
	d := time.Now().Add(-dur)
	jobsBeforeDate, err := job.GetJobsBeforeDate(instance, d)
	if err != nil && err != job.ErrNotFoundJob {
		return err
	}
	// Step 2: We also want to keep a minimum number of jobs for each state.
	// Jobs returned will be kept.
	lastsJobs := map[string]struct{}{}
	for _, w := range workers {
		jobs, err := job.GetLastsJobs(instance, w)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			lastsJobs[j.ID()] = struct{}{}
		}
	}

	// Step 3: cleaning.
	// - Removing jobs from the ids if they exists.
	// - Skipping worker types
	var finalJobs []*job.Job

	for _, j := range jobsBeforeDate {
		validWorker := false

		for _, wt := range workers {
			if j.WorkerType == wt {
				validWorker = true
				break
			}
		}
		// Check the job is not existing in the lasts jobs
		if validWorker {
			_, ok := lastsJobs[j.ID()]

			if !ok {
				finalJobs = append(finalJobs, j)
			}
		}
	}

	// Bulk-deleting the jobs
	jobsToDelete := make([]couchdb.Doc, len(finalJobs))
	for i, j := range finalJobs {
		jobsToDelete[i] = j
	}
	err = couchdb.BulkDeleteDocs(instance, consts.Jobs, jobsToDelete)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]int{"deleted": len(jobsToDelete)})
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
	router.DELETE("/purge", purgeJobs)
	router.GET("/:job-id", getJob)
}

func wrapJobsError(err error) error {
	switch err {
	case job.ErrNotFoundTrigger,
		job.ErrNotFoundJob,
		job.ErrUnknownWorker:
		return jsonapi.NotFound(err)
	case job.ErrUnknownTrigger:
		return jsonapi.InvalidAttribute("Type", err)
	}
	return err
}
