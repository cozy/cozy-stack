package jobs

import (
	"errors"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/sirupsen/logrus"
)

type (
	couchStorage struct {
		db couchdb.Database
	}

	// Job struct contains all the parameters of a job
	Job struct {
		// No mutex, a Job is expected to be used from only one goroutine at a time
		infos   *JobInfos
		storage *couchStorage
	}
)

func (c *couchStorage) Get(domain, jobID string) (*JobInfos, error) {
	var job JobInfos
	if err := couchdb.GetDoc(c.db, consts.Jobs, jobID, &job); err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, ErrNotFoundJob
		}
		return nil, err
	}
	if job.Domain != domain {
		return nil, ErrNotFoundJob
	}
	return &job, nil
}

func (c *couchStorage) Create(job *JobInfos) error {
	if err := couchdb.CreateDoc(c.db, job); err != nil {
		return err
	}
	if c == globalStorage {
		// Writing in couchdb should be enough to publish this event,
		// but it is not published on right domain, so we publish it again.
		realtime.GetHub().Publish(&realtime.Event{
			Verb:   realtime.EventCreate,
			Doc:    job.Clone(),
			Domain: job.Domain,
		})
	}
	return nil
}

func (c *couchStorage) Update(job *JobInfos) error {
	if err := couchdb.UpdateDoc(c.db, job); err != nil {
		return err
	}
	if c == globalStorage {
		// Writing in couchdb should be enough to publish this event,
		// but it is not published on right domain, so we publish it again.
		realtime.GetHub().Publish(&realtime.Event{
			Verb:   realtime.EventUpdate,
			Doc:    job.Clone(),
			Domain: job.Domain,
		})
	}
	return nil
}

// Domain returns the associated domain
func (j *Job) Domain() string {
	return j.infos.Domain
}

// Infos returns the associated job infos
func (j *Job) Infos() *JobInfos {
	return j.infos
}

// Logger returns a logger associated with the job domain
func (j *Job) Logger() *logrus.Entry {
	return logger.WithDomain(j.infos.Domain)
}

// AckConsumed sets the job infos state to Running an sends the new job infos
// on the channel.
func (j *Job) AckConsumed() error {
	job := *j.infos
	j.Logger().Debugf("[jobs] ack_consume %s ", job.ID())
	job.StartedAt = time.Now()
	job.State = Running
	j.infos = &job
	return j.persist()
}

// Ack sets the job infos state to Done an sends the new job infos on the
// channel.
func (j *Job) Ack() error {
	job := *j.infos
	j.Logger().Debugf("[jobs] ack %s ", job.ID())
	job.State = Done
	j.infos = &job
	return j.persist()
}

// Nack sets the job infos state to Errored, set the specified error has the
// error field and sends the new job infos on the channel.
func (j *Job) Nack(err error) error {
	job := *j.infos
	j.Logger().Debugf("[jobs] nack %s ", job.ID())
	job.State = Errored
	job.Error = err.Error()
	j.infos = &job
	return j.persist()
}

func (j *Job) persist() error {
	return j.storage.Update(j.infos)
}

// Marshal should not be used for a Job
func (j *Job) Marshal() ([]byte, error) {
	return nil, errors.New("should not be marshaled")
}

// Unmarshal should not be used for a Job
func (j *Job) Unmarshal() error {
	return errors.New("should not be unmarshaled")
}
