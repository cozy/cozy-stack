package share

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/messaging"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePublisher struct {
	requests []messaging.PublishRequest
	err      error
}

func (f *fakePublisher) Publish(_ context.Context, req messaging.PublishRequest) error {
	f.requests = append(f.requests, req)
	return f.err
}

func TestWorkerCollectedContacts(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	config.GetConfig().Contexts = map[string]interface{}{
		"collected-on": map[string]interface{}{"common_contacts": true},
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	inst, err := lifecycle.Create(&lifecycle.Options{
		Domain:      "alice-" + suffix + ".share.localhost",
		ContextName: "collected-on",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lifecycle.Destroy(inst.Domain) })
	require.NoError(t, instance.SetEmail(inst, "alice-"+suffix+"@acme.example"))

	// run executes the worker and tells if the job can be retried.
	run := func(publisher messaging.Publisher) (bool, error) {
		msg, err := job.NewMessage(&sharing.CollectedContactsMsg{Emails: []string{"dave@unknown.example"}})
		require.NoError(t, err)
		j := &job.Job{JobID: "1", Domain: inst.Domain, WorkerType: sharing.CollectedContactsWorker, Message: msg}
		ctx, cancel := job.NewTaskContext("0", j, inst)
		defer cancel()
		ctx.Publisher = publisher
		err = WorkerCollectedContacts(ctx)
		return !ctx.NoRetry(), err
	}

	published := &fakePublisher{}
	_, err = run(published)
	require.NoError(t, err)
	require.Len(t, published.requests, 1)
	assert.Equal(t, rabbitmq.ExchangeContactsCollected, published.requests[0].Exchange)
	payload := published.requests[0].Payload.(rabbitmq.CollectedContactsMessage)
	assert.Equal(t, inst.Email, payload.UserEmail)
	require.Len(t, payload.CollectedContacts, 1)

	// A publish error fails the job and allows a retry.
	failing := &fakePublisher{err: errors.New("broker down")}
	retry, err := run(failing)
	assert.ErrorIs(t, err, failing.err)
	assert.True(t, retry)

	// Without RabbitMQ, the job is not retried.
	retry, err = run(&fakePublisher{err: rabbitmq.ErrNotConfigured})
	assert.ErrorIs(t, err, rabbitmq.ErrNotConfigured)
	assert.False(t, retry)

	workers, err := job.GetWorkersList()
	require.NoError(t, err)
	var conf *job.WorkerConfig
	for _, w := range workers {
		if w.WorkerType == sharing.CollectedContactsWorker {
			conf = w
		}
	}
	require.NotNil(t, conf, "the worker must be registered")
	assert.True(t, conf.Reserved, "clients must not push this job")
	assert.Greater(t, conf.MaxExecCount, 1, "a failed publish must be retried")
}
