package realtime

import (
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

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
	c1 := h.Subscribe("testing", "io.cozy.testobject")
	c2 := h.Subscribe("testing", "io.cozy.testobject")
	c3 := h.SubscribeLocalAll()
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		for e := range c1.Read() {
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for e := range c2.Read() {
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for e := range c3.Read() {
			assert.Equal(t, "testing", e.Domain)
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	time.AfterFunc(1*time.Millisecond, func() {
		h.Publish(&Event{
			Domain: "testing",
			Doc: &testDoc{
				doctype: "io.cozy.testobject",
				id:      "foo",
			},
		})
	})

	wg.Wait()

	err := c1.Close()
	assert.NoError(t, err)
	err = c2.Close()
	assert.NoError(t, err)
	err = c3.Close()
	assert.NoError(t, err)

	err = c1.Close()
	assert.Error(t, err)

	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "nobodywillseeme",
		},
	})

	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "meneither",
		},
	})

	c4 := h.Subscribe("testing", "io.cozy.testobject")

	wg.Add(1)
	go func() {
		for e := range c4.Read() {
			assert.Equal(t, "bar", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	time.AfterFunc(1*time.Millisecond, func() {
		h.Publish(&Event{
			Domain: "testing",
			Doc: &testDoc{
				doctype: "io.cozy.testobject",
				id:      "bar",
			},
		})
	})

	wg.Wait()
}

func TestRedisRealtime(t *testing.T) {
	opt, err := redis.ParseURL("redis://localhost:6379/6")
	assert.NoError(t, err)
	client := redis.NewClient(opt)
	h := newRedisHub(client)
	c1 := h.Subscribe("testing", "io.cozy.testobject")
	c2 := h.Subscribe("testing", "io.cozy.testobject")
	c3 := h.SubscribeLocalAll()
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		for e := range c1.Read() {
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for e := range c2.Read() {
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		for e := range c3.Read() {
			assert.Equal(t, "testing", e.Domain)
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	time.AfterFunc(1*time.Millisecond, func() {
		h.Publish(&Event{
			Domain: "testing",
			Doc: &testDoc{
				doctype: "io.cozy.testobject",
				id:      "foo",
			},
		})
	})

	wg.Wait()

	err = c1.Close()
	assert.NoError(t, err)
	err = c2.Close()
	assert.NoError(t, err)
	err = c3.Close()
	assert.NoError(t, err)

	err = c1.Close()
	assert.Error(t, err)

	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "nobodywillseeme",
		},
	})

	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "meneither",
		},
	})

	time.Sleep(100 * time.Millisecond)
	c4 := h.Subscribe("testing", "io.cozy.testobject")

	wg.Add(1)
	go func() {
		for e := range c4.Read() {
			assert.Equal(t, "bar", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	time.AfterFunc(1*time.Millisecond, func() {
		h.Publish(&Event{
			Domain: "testing",
			Doc: &testDoc{
				doctype: "io.cozy.testobject",
				id:      "bar",
			},
		})
	})

	wg.Wait()
}
