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
	h := GetHub()
	c1 := h.Subscribe("testing", "io.cozy.testobject")
	c2 := h.Subscribe("testing", "io.cozy.testobject")
	c3 := h.SubscribeAll()
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
}
