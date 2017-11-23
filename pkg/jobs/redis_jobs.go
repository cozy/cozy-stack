package jobs

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis"
	multierror "github.com/hashicorp/go-multierror"
)

// redisPrefix is the prefix for jobs queues in redis.
const redisPrefix = "j/"

type redisBroker struct {
	client    *redis.Client
	queues    map[string]chan *Job
	nbWorkers int
	workers   []*Worker
	running   uint32
	closed    chan struct{}
}

// NewRedisBroker creates a new broker that will use redis to distribute
// the jobs among several cozy-stack processes.
func NewRedisBroker(nbWorkers int, client *redis.Client) Broker {
	return &redisBroker{
		client:    client,
		nbWorkers: nbWorkers,
		queues:    make(map[string]chan *Job),
		closed:    make(chan struct{}),
	}
}

// Start polling jobs from redis queues
func (b *redisBroker) Start(ws WorkersList) error {
	if !atomic.CompareAndSwapUint32(&b.running, 0, 1) {
		return ErrClosed
	}
	if b.nbWorkers <= 0 {
		return nil
	}

	setNbSlots(b.nbWorkers)
	joblog.Infof("Starting redis broker with %d workers", b.nbWorkers)

	var keys []string
	for workerType, conf := range ws {
		ch := make(chan *Job)
		w := &Worker{
			Type: workerType,
			Conf: conf,
		}
		b.queues[workerType] = ch
		b.workers = append(b.workers, w)
		keys = append(keys, redisPrefix+workerType)
		if err := w.Start(ch); err != nil {
			return err
		}
	}

	go b.pollLoop(keys)

	return nil
}

func (b *redisBroker) WorkersTypes() []string {
	types := make([]string, len(b.workers))
	for i, worker := range b.workers {
		types[i] = worker.Type
	}
	return types
}

func (b *redisBroker) Shutdown(ctx context.Context) error {
	if !atomic.CompareAndSwapUint32(&b.running, 1, 0) {
		return ErrClosed
	}
	if b.nbWorkers <= 0 {
		return nil
	}

	fmt.Print("  shutting down redis broker...")
	defer b.client.Close()

	select {
	case <-ctx.Done():
		fmt.Println("failed:", ctx.Err())
		return ctx.Err()
	case <-b.closed:
	}

	errs := make(chan error)
	for _, w := range b.workers {
		go func(w *Worker) { errs <- w.Shutdown(ctx) }(w)
	}

	var errm error
	for i := 0; i < len(b.workers); i++ {
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

var redisBRPopTimeout = 30 * time.Second

func (b *redisBroker) pollLoop(keys []string) {
	defer func() {
		b.closed <- struct{}{}
	}()

	for {
		if atomic.LoadUint32(&b.running) == 0 {
			return
		}

		results, err := b.client.BRPop(redisBRPopTimeout, keys...).Result()
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

		workerType := key[len(redisPrefix):]
		ch, ok := b.queues[workerType]
		if !ok {
			joblog.Warnf("Unknown workerType: %s", workerType)
			continue
		}

		domain, jobID := parts[0], parts[1]
		job, err := Get(domain, jobID)
		if err != nil {
			joblog.Warnf("Cannot find job %s on domain %s: %s", parts[1], parts[0], err)
			continue
		}

		ch <- job
	}
}

// PushJob will produce a new Job with the given options and enqueue the job in
// the proper queue.
func (b *redisBroker) PushJob(req *JobRequest) (*Job, error) {
	if atomic.LoadUint32(&b.running) == 0 {
		return nil, ErrClosed
	}

	job := NewJob(req)
	if err := job.Create(); err != nil {
		return nil, err
	}

	key := redisPrefix + job.WorkerType
	val := job.Domain + "/" + job.JobID
	if err := b.client.LPush(key, val).Err(); err != nil {
		return nil, err
	}

	return job, nil
}

// QueueLen returns the size of the number of elements in queue of the
// specified worker type.
func (b *redisBroker) QueueLen(workerType string) (int, error) {
	key := redisPrefix + workerType
	l, err := b.client.LLen(key).Result()
	return int(l), err
}
