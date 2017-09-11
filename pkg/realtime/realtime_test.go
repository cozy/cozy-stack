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
	c1 := h.Subscriber("testing")
	c2 := h.Subscriber("testing")
	c3 := h.SubscribeLocalAll()
	wg := sync.WaitGroup{}

	err := c1.Subscribe("io.cozy.testobject")
	assert.NoError(t, err)
	err = c2.Subscribe("io.cozy.testobject")
	assert.NoError(t, err)

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

	time.Sleep(1 * time.Millisecond)

	c4 := h.Subscriber("testing")
	err = c4.Subscribe("io.cozy.testobject")
	assert.NoError(t, err)
	err = c4.Subscribe("io.cozy.testobject2")
	assert.NoError(t, err)

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
		h.Publish(&Event{
			Domain: "testing",
			Doc: &testDoc{
				doctype: "io.cozy.testobject",
				id:      "bar",
			},
		})
	})
	time.AfterFunc(20*time.Millisecond, func() {
		h.Publish(&Event{
			Domain: "testing",
			Doc: &testDoc{
				doctype: "io.cozy.testobject2",
				id:      "baz",
			},
		})
	})

	wg.Wait()
}

func TestWatch(t *testing.T) {
	h := newMemHub()
	c1 := h.Subscriber("testing")
	wg := sync.WaitGroup{}

	err := c1.Watch("io.cozy.testobject", "id1")
	assert.NoError(t, err)
	err = c1.Watch("io.cozy.testobject", "id2")
	assert.NoError(t, err)

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
	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "not-id1-and-not-id2",
		},
	})

	time.Sleep(1 * time.Millisecond)
	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "id1",
		},
	})

	time.Sleep(1 * time.Millisecond)
	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "id2",
		},
	})

	wg.Wait()

	err = c1.Subscribe("io.cozy.testobject")
	assert.NoError(t, err)

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
	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "id1",
		},
	})

	time.Sleep(1 * time.Millisecond)
	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "id2",
		},
	})

	time.Sleep(1 * time.Millisecond)
	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "id3",
		},
	})

	wg.Wait()

	err = c1.Close()
	assert.NoError(t, err)
}

func TestRedisRealtime(t *testing.T) {
	opt, err := redis.ParseURL("redis://localhost:6379/6")
	assert.NoError(t, err)
	client := redis.NewClient(opt)
	h := newRedisHub(client)
	c1 := h.Subscriber("testing")
	c2 := h.Subscriber("testing")
	c3 := h.SubscribeLocalAll()
	wg := sync.WaitGroup{}

	err = c1.Subscribe("io.cozy.testobject")
	assert.NoError(t, err)
	err = c2.Subscribe("io.cozy.testobject")
	assert.NoError(t, err)

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
	time.Sleep(100 * time.Millisecond)

	h.Publish(&Event{
		Domain: "testing",
		Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "meneither",
		},
	})
	time.Sleep(100 * time.Millisecond)

	c4 := h.Subscriber("testing")
	err = c4.Subscribe("io.cozy.testobject")
	assert.NoError(t, err)
	err = c4.Subscribe("io.cozy.testobject2")
	assert.NoError(t, err)

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
		h.Publish(&Event{
			Domain: "testing",
			Doc: &testDoc{
				doctype: "io.cozy.testobject",
				id:      "bar",
			},
		})
	})
	time.AfterFunc(20*time.Millisecond, func() {
		h.Publish(&Event{
			Domain: "testing",
			Doc: &testDoc{
				doctype: "io.cozy.testobject2",
				id:      "baz",
			},
		})
	})

	wg.Wait()
}
