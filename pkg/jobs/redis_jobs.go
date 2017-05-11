package jobs

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/go-redis/redis"
)

const redisPrefix = "j:"

type redisBroker struct {
	client *redis.Client
}

// NewRedisBroker creates a new broker that will use redis to distribute
// the jobs among several cozy-stack processes.
func NewRedisBroker(client *redis.Client) Broker {
	return &redisBroker{
		client: client,
	}
}

// PushJob will produce a new Job with the given options and enqueue the job in
// the proper queue.
func (b *redisBroker) PushJob(req *JobRequest) (*JobInfos, <-chan *JobInfos, error) {
	infos := NewJobInfos(req)
	db := couchdb.SimpleDatabasePrefix(infos.Domain)
	if err := couchdb.CreateDoc(db, infos); err != nil {
		return nil, nil, err
	}

	key := redisPrefix + infos.WorkerType
	val := infos.Domain + "/" + infos.JobID
	if err := b.client.LPush(key, val).Err(); err != nil {
		return nil, nil, err
	}

	// TODO remove the chan from the signature of PushJob
	jobch := make(chan *JobInfos, 2)
	return infos, jobch, nil
}

// QueueLen returns the size of the number of elements in queue of the
// specified worker type.
func (b *redisBroker) QueueLen(workerType string) (int, error) {
	key := redisPrefix + workerType
	l, err := b.client.LLen(key).Result()
	return int(l), err
}

// GetJobInfos returns the informations about a job.
func (b *redisBroker) GetJobInfos(domain, jobID string) (*JobInfos, error) {
	var infos JobInfos
	db := couchdb.SimpleDatabasePrefix(domain)
	if err := couchdb.GetDoc(db, consts.Jobs, jobID, &infos); err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, ErrNotFoundJob
		}
		return nil, err
	}
	return &infos, nil
}
