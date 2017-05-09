package jobs

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// Storage is an interface providing the methods to create, update and fetch a
// job.
type Storage interface {
	Get(domain, jobID string) (*JobInfos, error)
	Create(job *JobInfos) error
	Update(job *JobInfos) error
}

// GlobalStorage is the global job persistence layer used thoughout the stack.
var GlobalStorage Storage = &couchStorage{couchdb.GlobalJobsDB}

type couchStorage struct {
	db couchdb.Database
}

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
	return couchdb.CreateDoc(c.db, job)
}

func (c *couchStorage) Update(job *JobInfos) error {
	return couchdb.UpdateDoc(c.db, job)
}
