package realtime

import (
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

var testingDB = prefixer.NewPrefixer(0, "testing", "testing")

type testDoc struct {
	id      string
	doctype string
}

func (t *testDoc) ID() string      { return t.id }
func (t *testDoc) DocType() string { return t.doctype }
func (t *testDoc) MarshalJSON() ([]byte, error) {
	j := `{"_id":"` + t.id + `", "_type":"` + t.doctype + `"}`
	return []byte(j), nil
}

func TestMemRealtime(t *testing.T) {
	h := newMemHub()
	c1 := h.Subscriber(testingDB)
	c2 := h.Subscriber(testingDB)
	c3 := h.SubscribeFirehose()
	wg := sync.WaitGroup{}

	c1.Subscribe("io.cozy.testobject")
	c2.Subscribe("io.cozy.testobject")

	wg.Add(1)
	go func() {
		for e := range c1.Channel {
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for e := range c2.Channel {
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for e := range c3.Channel {
			assert.Equal(t, "testing", e.Domain)
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	time.AfterFunc(10*time.Millisecond, func() {
		h.Publish(testingDB, EventCreate, &testDoc{doctype: "io.cozy.testobject", id: "foo"}, nil)
	})

	wg.Wait()

	c1.Close()
	c2.Close()
	c3.Close()

	c1.Close()

	h.Publish(testingDB, EventCreate, &testDoc{doctype: "io.cozy.testobject", id: "nobodywillseeme"}, nil)
	h.Publish(testingDB, EventCreate, &testDoc{doctype: "io.cozy.testobject", id: "meneither"}, nil)
	time.Sleep(1 * time.Millisecond)

	c4 := h.Subscriber(testingDB)
	c4.Subscribe("io.cozy.testobject")
	c4.Subscribe("io.cozy.testobject2")
	defer c4.Close()

	wg.Add(2)
	go func() {
		expected := "bar"
		for e := range c4.Channel {
			assert.Equal(t, expected, e.Doc.ID())
			wg.Done()
			if expected == "baz" {
				break
			}
			expected = "baz"
		}
	}()

	time.AfterFunc(10*time.Millisecond, func() {
		h.Publish(testingDB, EventCreate, &testDoc{
			doctype: "io.cozy.testobject",
			id:      "bar",
		}, nil)
	})
	time.AfterFunc(20*time.Millisecond, func() {
		h.Publish(testingDB, EventCreate, &testDoc{
			doctype: "io.cozy.testobject2",
			id:      "baz",
		}, nil)
	})

	wg.Wait()
}

func TestWatch(t *testing.T) {
	h := newMemHub()
	c1 := h.Subscriber(testingDB)
	wg := sync.WaitGroup{}

	c1.Watch("io.cozy.testobject", "id1")
	c1.Watch("io.cozy.testobject", "id2")

	wg.Add(1)
	go func() {
		for e := range c1.Channel {
			assert.Equal(t, "id1", e.Doc.ID())
			break
		}
		for e := range c1.Channel {
			assert.Equal(t, "id2", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	time.Sleep(1 * time.Millisecond)
	h.Publish(testingDB, EventCreate, &testDoc{
		doctype: "io.cozy.testobject",
		id:      "not-id1-and-not-id2",
	}, nil)

	time.Sleep(1 * time.Millisecond)
	h.Publish(testingDB, EventCreate, &testDoc{
		doctype: "io.cozy.testobject",
		id:      "id1",
	}, nil)

	time.Sleep(1 * time.Millisecond)
	h.Publish(testingDB, EventCreate, &testDoc{
		doctype: "io.cozy.testobject",
		id:      "id2",
	}, nil)

	wg.Wait()

	c1.Subscribe("io.cozy.testobject")

	wg.Add(1)
	go func() {
		for e := range c1.Channel {
			assert.Equal(t, "id1", e.Doc.ID())
			break
		}
		for e := range c1.Channel {
			assert.Equal(t, "id2", e.Doc.ID())
			break
		}
		for e := range c1.Channel {
			assert.Equal(t, "id3", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	time.Sleep(1 * time.Millisecond)
	h.Publish(testingDB, EventCreate, &testDoc{
		doctype: "io.cozy.testobject",
		id:      "id1",
	}, nil)

	time.Sleep(1 * time.Millisecond)
	h.Publish(testingDB, EventCreate, &testDoc{
		doctype: "io.cozy.testobject",
		id:      "id2",
	}, nil)

	time.Sleep(1 * time.Millisecond)
	h.Publish(testingDB, EventCreate, &testDoc{
		doctype: "io.cozy.testobject",
		id:      "id3",
	}, nil)

	wg.Wait()

	c1.Close()
}

func TestSubscribeWatchUnwatch(t *testing.T) {
	h := newMemHub()
	sub := h.Subscriber(testingDB)
	defer sub.Close()

	sub.Subscribe("io.cozy.testobject")
	time.Sleep(1 * time.Millisecond)
	sub.Watch("io.cozy.testobject", "id1")
	time.Sleep(1 * time.Millisecond)
	sub.Unwatch("io.cozy.testobject", "id1")
	time.Sleep(1 * time.Millisecond)

	h.Publish(testingDB, EventCreate, &testDoc{
		doctype: "io.cozy.testobject",
		id:      "id2",
	}, nil)
	e := <-sub.Channel
	assert.Equal(t, "id2", e.Doc.ID())
}

func TestRedisRealtime(t *testing.T) {
	if testing.Short() {
		t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
	}

	opt, err := redis.ParseURL("redis://localhost:6379/6")
	assert.NoError(t, err)
	client := redis.NewClient(opt)
	h := newRedisHub(client)
	c1 := h.Subscriber(testingDB)
	c2 := h.Subscriber(testingDB)
	c3 := h.SubscribeFirehose()
	wg := sync.WaitGroup{}

	c1.Subscribe("io.cozy.testobject")
	c2.Subscribe("io.cozy.testobject")

	wg.Add(1)
	go func() {
		for e := range c1.Channel {
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for e := range c2.Channel {
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for e := range c3.Channel {
			assert.Equal(t, "testing", e.Domain)
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	time.AfterFunc(10*time.Millisecond, func() {
		h.Publish(testingDB, EventCreate, &testDoc{
			doctype: "io.cozy.testobject",
			id:      "foo",
		}, nil)
	})

	wg.Wait()

	c1.Close()
	c2.Close()
	c3.Close()

	c1.Close()

	h.Publish(testingDB, EventCreate, &testDoc{
		doctype: "io.cozy.testobject",
		id:      "nobodywillseeme",
	}, nil)
	time.Sleep(100 * time.Millisecond)

	h.Publish(testingDB, EventCreate, &testDoc{
		doctype: "io.cozy.testobject",
		id:      "meneither",
	}, nil)
	time.Sleep(100 * time.Millisecond)

	c4 := h.Subscriber(testingDB)
	c4.Subscribe("io.cozy.testobject")
	c4.Subscribe("io.cozy.testobject2")
	defer c4.Close()

	wg.Add(2)
	go func() {
		expected := "bar"
		for e := range c4.Channel {
			assert.Equal(t, expected, e.Doc.ID())
			wg.Done()
			if expected == "baz" {
				break
			}
			expected = "baz"
		}
	}()

	time.AfterFunc(10*time.Millisecond, func() {
		h.Publish(testingDB, EventCreate, &testDoc{
			doctype: "io.cozy.testobject",
			id:      "bar",
		}, nil)
	})
	time.AfterFunc(20*time.Millisecond, func() {
		h.Publish(testingDB, EventCreate, &testDoc{
			doctype: "io.cozy.testobject2",
			id:      "baz",
		}, nil)
	})

	wg.Wait()
}
