package jobs

import (
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/go-redis/redis"
)

var joblog = logger.WithNamespace("redis-job")

const redisPrefix = "j/"

type redisBroker struct {
	client  *redis.Client
	queues  map[string]chan Job
	running bool
}

// NewRedisBroker creates a new broker that will use redis to distribute
// the jobs among several cozy-stack processes.
func NewRedisBroker(nbWorkers int, client *redis.Client) Broker {
	broker := &redisBroker{
		client: client,
	}
	if nbWorkers > 0 {
		setNbSlots(nbWorkers)
		broker.Start(GetWorkersList())
	}
	return broker
}

// Start polling jobs from redis queues
func (b *redisBroker) Start(ws WorkersList) {
	b.queues = make(map[string]chan Job)
	var keys []string
	for workerType, conf := range ws {
		ch := make(chan Job)
		b.queues[workerType] = ch
		w := &Worker{
			Type: workerType,
			Conf: conf,
		}
		w.Start(ch)
		keys = append(keys, redisPrefix+workerType)
	}
	b.running = true
	go b.pollLoop(keys)
}

func (b *redisBroker) Stop() {
	b.running = false
}

var redisBRPopTimeout = 30 * time.Second

func (b *redisBroker) pollLoop(keys []string) {
	for {
		if !b.running {
			return
		}
		results, err := b.client.BRPop(redisBRPopTimeout, keys...).Result()
		if err != nil || len(results) < 2 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		workerType := results[0][len(redisPrefix):]
		ch, ok := b.queues[workerType]
		if !ok {
			joblog.Warnf("Unknown workerType: %s", workerType)
			continue
		}

		parts := strings.SplitN(results[1], "/", 2)
		if len(parts) != 2 {
			joblog.Warnf("Invalid key %s", results[1])
			continue
		}
		infos, err := b.GetJobInfos(parts[0], parts[1])
		if err != nil {
			joblog.Warnf("Cannot find job %s on domain %s: %s", parts[1], parts[0], err)
			continue
		}

		job := Job{
			infos: infos,
			storage: &couchStorage{
				db: couchdb.SimpleDatabasePrefix(parts[0]),
			},
		}
		ch <- job
	}
}

// PushJob will produce a new Job with the given options and enqueue the job in
// the proper queue.
func (b *redisBroker) PushJob(req *JobRequest) (*JobInfos, error) {
	infos := NewJobInfos(req)
	db := couchdb.SimpleDatabasePrefix(infos.Domain)
	if err := couchdb.CreateDoc(db, infos); err != nil {
		return nil, err
	}

	key := redisPrefix + infos.WorkerType
	val := infos.Domain + "/" + infos.JobID
	if err := b.client.LPush(key, val).Err(); err != nil {
		return nil, err
	}
	return infos, nil
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
