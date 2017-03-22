package realtime

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testDoc struct {
	id      string
	rev     string
	doctype string
}

func (t *testDoc) ID() string      { return t.id }
func (t *testDoc) Rev() string     { return t.rev }
func (t *testDoc) DocType() string { return t.doctype }

func TestRealtime(t *testing.T) {
	h := InstanceHub("testing")
	main := MainHub()
	c := h.Subscribe("io.cozy.testobject")
	c2 := h.Subscribe("io.cozy.testobject")
	c3 := main.Subscribe("io.cozy.testobject")
	wg := sync.WaitGroup{}

	assert.Panics(t, func() {
		main.Publish(&Event{Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "foo",
		}})
	})

	wg.Add(1)
	go func() {
		for e := range c.Read() {
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
			assert.Equal(t, "testing", e.Instance)
			assert.Equal(t, "foo", e.Doc.ID())
			break
		}
		wg.Done()
	}()

	time.AfterFunc(1*time.Millisecond, func() {
		h.Publish(&Event{Doc: &testDoc{
			doctype: "io.cozy.testobject",
			id:      "foo",
		}})
	})

	wg.Wait()

	err := c.Close()
	assert.NoError(t, err)
	err = c2.Close()
	assert.NoError(t, err)

	err = c.Close()
	assert.Error(t, err)

	h.Publish(&Event{Doc: &testDoc{
		doctype: "io.cozy.testobject",
		id:      "nobodywillseeme",
	}})

	h.Publish(&Event{Doc: &testDoc{
		doctype: "io.cozy.testobject",
		id:      "meneither",
	}})

}
