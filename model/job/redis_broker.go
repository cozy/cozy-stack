package job

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/go-redis/redis"
	multierror "github.com/hashicorp/go-multierror"
)

const (
	// redisPrefix is the prefix for jobs queues in redis.
	redisPrefix = "j/"
	// redisHighPrioritySuffix suffix is the suffix used for prioritized queue.
	redisHighPrioritySuffix = "/p0"
)

type redisBroker struct {
	client         redis.UniversalClient
	workers        []*Worker
	workersRunning []*Worker
	workersTypes   []string
	running        uint32
	closed         chan struct{}
}

// NewRedisBroker creates a new broker that will use redis to distribute
// the jobs among several cozy-stack processes.
func NewRedisBroker(client redis.UniversalClient) Broker {
	return &redisBroker{
		client: client,
		closed: make(chan struct{}),
	}
}

// StartWorkers polling jobs from redis queues
func (b *redisBroker) StartWorkers(ws WorkersList) error {
	if !atomic.CompareAndSwapUint32(&b.running, 0, 1) {
		return ErrClosed
	}

	for _, conf := range ws {
		b.workersTypes = append(b.workersTypes, conf.WorkerType)
		w := NewWorker(conf)
		b.workers = append(b.workers, w)
		if conf.Concurrency <= 0 {
			continue
		}
		b.workersRunning = append(b.workersRunning, w)
		ch := make(chan *Job)
		if err := w.Start(ch); err != nil {
			return err
		}
		go b.pollLoop(redisPrefix+conf.WorkerType, ch)
	}

	if len(b.workersRunning) > 0 {
		joblog.Infof("Started redis broker for %d workers type", len(b.workersRunning))
	}

	// XXX for retro-compat
	if slots := config.GetConfig().Jobs.NbWorkers; len(b.workersRunning) > 0 && slots > 0 {
		joblog.Warnf("Limiting the number of total concurrent workers to %d", slots)
		joblog.Warnf("Please update your configuration file to avoid a hard limit")
		setNbSlots(slots)
	}

	return nil
}

func (b *redisBroker) WorkersTypes() []string {
	return b.workersTypes
}

func (b *redisBroker) ShutdownWorkers(ctx context.Context) error {
	if !atomic.CompareAndSwapUint32(&b.running, 1, 0) {
		return ErrClosed
	}
	if len(b.workersRunning) == 0 {
		return nil
	}

	fmt.Print("  shutting down redis broker...")
	defer b.client.Close()

	for i := 0; i < len(b.workersRunning); i++ {
		select {
		case <-ctx.Done():
			fmt.Println("failed:", ctx.Err())
			return ctx.Err()
		case <-b.closed:
		}
	}

	errs := make(chan error)
	for _, w := range b.workersRunning {
		go func(w *Worker) { errs <- w.Shutdown(ctx) }(w)
	}

	var errm error
	for i := 0; i < len(b.workersRunning); i++ {
		if err := <-errs; err != nil {
			errm = multierror.Append(errm, err)
		}
	}

	if errm != nil {
		fmt.Println("failed: ", errm)
	} else {
		fmt.Println("ok")
	}
	return errm
}

var redisBRPopTimeout = 10 * time.Second

func (b *redisBroker) pollLoop(key string, ch chan<- *Job) {
	defer func() {
		b.closed <- struct{}{}
	}()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		if atomic.LoadUint32(&b.running) == 0 {
			return
		}

		// The brpop redis command will always take elements in priority from the
		// first key containing elements at the call. By always priorizing the
		// manual queue, this would cause a starvation for our main queue if too
		// many "manual" jobs are pushed. By randomizing the order we make sure we
		// avoid such starvation. For one in three call, the main queue is
		// selected.
		keyP0 := key + redisHighPrioritySuffix
		keyP1 := key
		if rng.Intn(3) == 0 {
			keyP1, keyP0 = keyP0, keyP1
		}
		results, err := b.client.BRPop(redisBRPopTimeout, keyP0, keyP1).Result()
		if err != nil || len(results) < 2 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		key := results[0]
		val := results[1]
		if len(key) < len(redisPrefix) {
			joblog.Warnf("Invalid key %s", key)
			continue
		}

		parts := strings.SplitN(val, "/", 2)
		if len(parts) != 2 {
			joblog.Warnf("Invalid val %s", val)
			continue
		}

		prefix, jobID := parts[0], parts[1]
		job, err := Get(prefixer.NewPrefixer("", prefix), jobID)
		if err != nil {
			joblog.Warnf("Cannot find job %s on domain %s: %s", parts[1], parts[0], err)
			continue
		}

		ch <- job
	}
}

// PushJob will produce a new Job with the given options and enqueue the job in
// the proper queue.
func (b *redisBroker) PushJob(db prefixer.Prefixer, req *JobRequest) (*Job, error) {
	if atomic.LoadUint32(&b.running) == 0 {
		return nil, ErrClosed
	}

	var worker *Worker
	for _, w := range b.workers {
		if w.Type == req.WorkerType {
			worker = w
			break
		}
	}
	if worker == nil {
		return nil, ErrUnknownWorker
	}
	if worker.Conf.AdminOnly && !req.Admin {
		return nil, ErrUnknownWorker
	}

	job := NewJob(db, req)
	if worker.Conf.BeforeHook != nil {
		ok, err := worker.Conf.BeforeHook(job)
		if err != nil {
			return nil, err
		}
		if !ok {
			return job, nil
		}
	}

	if err := job.Create(); err != nil {
		return nil, err
	}

	key := redisPrefix + job.WorkerType
	val := job.DBPrefix() + "/" + job.JobID

	// When the job is manual, it is being pushed in a specific prioritized
	// queue.
	if job.Manual {
		key += redisHighPrioritySuffix
	}

	if err := b.client.LPush(key, val).Err(); err != nil {
		return nil, err
	}

	return job, nil
}

// QueueLen returns the size of the number of elements in queue of the
// specified worker type.
func (b *redisBroker) WorkerQueueLen(workerType string) (int, error) {
	key := redisPrefix + workerType
	l1, err := b.client.LLen(key).Result()
	if err != nil {
		return 0, err
	}
	l2, err := b.client.LLen(key + redisHighPrioritySuffix).Result()
	if err != nil {
		return 0, err
	}
	return int(l1 + l2), nil
}
