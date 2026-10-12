package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskContextPublisher struct {
	requests []messaging.PublishRequest
}

func (p *taskContextPublisher) Publish(_ context.Context, req messaging.PublishRequest) error {
	p.requests = append(p.requests, req)
	return nil
}

func TestTaskContextPublisher(t *testing.T) {
	j := &job.Job{JobID: "publisher-test", Domain: "cozy.localhost"}
	ctx, cancel := job.NewTaskContext("worker", j, nil)
	defer cancel()
	assert.Nil(t, ctx.Publisher)

	pub := new(taskContextPublisher)
	ctx.Publisher = pub
	timed, cancelTimeout := ctx.WithTimeout(time.Second)
	defer cancelTimeout()
	require.Same(t, pub, timed.Publisher)
	withCookie := timed.WithCookie("cookie")
	require.Same(t, pub, withCookie.Publisher)

	req := messaging.PublishRequest{Exchange: "activity", RoutingKey: "test.ready", Payload: "ready"}
	require.NoError(t, withCookie.Publisher.Publish(withCookie, req))
	assert.Equal(t, []messaging.PublishRequest{req}, pub.requests)
}
