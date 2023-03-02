package limits

import (
	"sync"
	"time"
)

var counterCleanInterval = 1 * time.Second

type memRef struct {
	val int64
	exp time.Time
}

// InMemory implementation ofr [Counter].
type InMemory struct {
	mu   sync.Mutex
	vals map[string]*memRef
}

// NewInMemory returns a in-memory counter.
func NewInMemory() *InMemory {
	counter := &InMemory{vals: make(map[string]*memRef)}

	go counter.cleaner()

	return counter
}

func (i *InMemory) cleaner() {
	for range time.Tick(counterCleanInterval) {
		i.mu.Lock()

		now := time.Now()
		for k, v := range i.vals {
			if now.After(v.exp) {
				delete(i.vals, k)
			}
		}

		i.mu.Unlock()
	}
}

func (i *InMemory) Increment(key string, timeLimit time.Duration) (int64, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, ok := i.vals[key]; !ok {
		i.vals[key] = &memRef{
			val: 0,
			exp: time.Now().Add(timeLimit),
		}
	}
	i.vals[key].val++
	return i.vals[key].val, nil
}

func (i *InMemory) Reset(key string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	delete(i.vals, key)

	return nil
}
