package orgdrive

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/tests/testutils"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePublisher struct {
	requests []rabbitmq.PublishRequest
	err      error
}

func (p *fakePublisher) Publish(_ context.Context, req rabbitmq.PublishRequest) error {
	if p.err != nil {
		return p.err
	}
	p.requests = append(p.requests, req)
	return nil
}

func (p *fakePublisher) ready(t *testing.T, i int) ReadyData {
	t.Helper()
	require.Greater(t, len(p.requests), i)
	req := p.requests[i]
	assert.Equal(t, rabbitmq.ExchangeActivity, req.Exchange)
	assert.Equal(t, rabbitmq.RoutingKeyOrgDriveReady, req.RoutingKey)
	event := req.Payload.(rabbitmq.CloudEvent)
	assert.Equal(t, "twake://drive", event.Source)
	return event.Data.(ReadyData)
}

func TestDriveQueueUsesTheOrgDriveHandler(t *testing.T) {
	svc := new(rabbitmq.RabbitMQService)
	specs := svc.BuildExchangeSpecs([]config.RabbitExchange{{
		Name:   rabbitmq.ExchangeDrive,
		Kind:   "topic",
		Queues: []config.RabbitQueue{{Name: rabbitmq.QueueDriveCommands}},
	}})

	require.Len(t, specs, 1)
	require.Len(t, specs[0].Queues, 1)
	assert.Equal(t, NewHandler(svc), specs[0].Queues[0].Handler)
}

func TestParseCommand(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{
			"organizationId": "org123",
			"reference":      map[string]any{"source": "twake-space", "id": "s1"},
			"revision":       1,
			"state":          "created",
			"name":           "Product launch",
			"members":        []any{map[string]any{"email": "alice@example.com", "role": "write"}},
			"groups":         []any{map[string]any{"id": "engineering", "role": "read"}},
		}
	}
	cmd, err := ParseCommand(mustJSON(t, valid()))
	require.NoError(t, err)
	assert.Equal(t, Reference{Source: "twake-space", ID: "s1"}, cmd.Reference)

	for name, change := range map[string]func(map[string]any){
		"no organization": func(m map[string]any) { delete(m, "organizationId") },
		"no reference id": func(m map[string]any) { m["reference"] = map[string]any{"source": "twake-space"} },
		"zero revision":   func(m map[string]any) { m["revision"] = 0 },
		"unknown state":   func(m map[string]any) { m["state"] = "archived" },
		"created no name": func(m map[string]any) { m["name"] = " " },
		"unknown role": func(m map[string]any) {
			m["members"] = []any{map[string]any{"email": "a@example.com", "role": "admin"}}
		},
		"member no email":    func(m map[string]any) { m["members"] = []any{map[string]any{"role": "read"}} },
		"group unknown role": func(m map[string]any) { m["groups"] = []any{map[string]any{"id": "g", "role": "owner"}} },
	} {
		t.Run(name, func(t *testing.T) {
			m := valid()
			change(m)
			_, err := ParseCommand(mustJSON(t, m))
			assert.ErrorIs(t, err, rabbitmq.ErrMalformed)
		})
	}

	_, err = ParseCommand([]byte("{"))
	assert.ErrorIs(t, err, rabbitmq.ErrMalformed)
}

func TestHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	ctx := context.Background()

	t.Run("CreatesTheDriveAndRepliesWithItsID", func(t *testing.T) {
		org := newOrgInstance(t)
		ref := "space-" + utils.RandomString(8)
		pub := &fakePublisher{}

		require.NoError(t, NewHandler(pub).Handle(ctx, ensure(t, org, ref, 1, "Product launch")))

		s := onlyDrive(t, org)
		assert.True(t, s.OrgDrive)
		assert.Equal(t, "Product launch", s.Description)
		assert.Equal(t, &sharing.Reference{Source: "twake-space", ID: ref, Revision: 1, Ready: true}, s.Reference)
		assert.Equal(t, "/Product launch", rootDir(t, org, s).Fullpath)

		data := pub.ready(t, 0)
		assert.Equal(t, Reference{Source: "twake-space", ID: ref}, data.Reference)
		assert.Equal(t, int64(1), data.Revision)
		assert.Equal(t, s.SID, data.Drive.ID)
		assert.Equal(t, org.OrgID, pub.requests[0].Payload.(rabbitmq.CloudEvent).TwakeOrg)
	})

	t.Run("RedeliveryRepliesAgainWithTheSameDrive", func(t *testing.T) {
		org := newOrgInstance(t)
		ref := "space-" + utils.RandomString(8)
		pub := &fakePublisher{}
		h := NewHandler(pub)

		require.NoError(t, h.Handle(ctx, ensure(t, org, ref, 1, "Roadmap")))
		require.NoError(t, h.Handle(ctx, ensure(t, org, ref, 1, "Roadmap")))

		s := onlyDrive(t, org)
		require.Len(t, pub.requests, 2)
		assert.Equal(t, s.SID, pub.ready(t, 1).Drive.ID)
	})

	t.Run("NewerRevisionRenamesTheDrive", func(t *testing.T) {
		org := newOrgInstance(t)
		ref := "space-" + utils.RandomString(8)
		h := NewHandler(&fakePublisher{})

		require.NoError(t, h.Handle(ctx, ensure(t, org, ref, 1, "Sales")))
		require.NoError(t, h.Handle(ctx, ensure(t, org, ref, 2, "Sales EMEA")))

		s := onlyDrive(t, org)
		assert.Equal(t, "Sales EMEA", s.Description)
		assert.Equal(t, int64(2), s.Reference.Revision)
		assert.Equal(t, "/Sales EMEA", rootDir(t, org, s).Fullpath)
	})

	t.Run("OlderRevisionIsIgnored", func(t *testing.T) {
		org := newOrgInstance(t)
		ref := "space-" + utils.RandomString(8)
		pub := &fakePublisher{}
		h := NewHandler(pub)

		require.NoError(t, h.Handle(ctx, ensure(t, org, ref, 5, "Legal")))
		require.NoError(t, h.Handle(ctx, ensure(t, org, ref, 4, "Old name")))

		s := onlyDrive(t, org)
		assert.Equal(t, "Legal", s.Description)
		assert.Len(t, pub.requests, 1)
	})

	t.Run("TakenNameGetsASuffix", func(t *testing.T) {
		org := newOrgInstance(t)
		_, err := vfs.Mkdir(org.VFS(), "/Finance", nil)
		require.NoError(t, err)

		require.NoError(t, NewHandler(&fakePublisher{}).Handle(ctx, ensure(t, org, "space-"+utils.RandomString(8), 1, "Finance")))

		assert.Equal(t, "/Finance (2)", rootDir(t, org, onlyDrive(t, org)).Fullpath)
	})

	t.Run("TrashedDriveIsReplaced", func(t *testing.T) {
		org := newOrgInstance(t)
		ref := "space-" + utils.RandomString(8)
		pub := &fakePublisher{}
		h := NewHandler(pub)

		require.NoError(t, h.Handle(ctx, ensure(t, org, ref, 1, "HR")))
		first := onlyDrive(t, org)
		_, err := vfs.TrashDir(org.VFS(), rootDir(t, org, first))
		require.NoError(t, err)

		require.NoError(t, h.Handle(ctx, ensure(t, org, ref, 2, "HR")))

		assert.NotEqual(t, first.SID, pub.ready(t, 1).Drive.ID)
	})

	t.Run("FailedReplyFailsTheDeliveryAndLeavesTheDriveNotReady", func(t *testing.T) {
		org := newOrgInstance(t)
		ref := "space-" + utils.RandomString(8)
		pub := &fakePublisher{err: errors.New("broker down")}

		require.Error(t, NewHandler(pub).Handle(ctx, ensure(t, org, ref, 1, "Ops")))
		assert.False(t, onlyDrive(t, org).Reference.Ready)

		pub.err = nil
		require.NoError(t, NewHandler(pub).Handle(ctx, ensure(t, org, ref, 1, "Ops")))
		assert.True(t, onlyDrive(t, org).Reference.Ready)
	})

	t.Run("UnknownOrganizationIsRetried", func(t *testing.T) {
		d := amqp.Delivery{RoutingKey: rabbitmq.RoutingKeyOrgDriveEnsure, Body: mustJSON(t, map[string]any{
			"organizationId": "nope" + strings.ToLower(utils.RandomString(8)),
			"reference":      map[string]any{"source": "twake-space", "id": "s1"},
			"revision":       1,
			"state":          "created",
			"name":           "X",
		})}
		err := NewHandler(&fakePublisher{}).Handle(ctx, d)
		require.Error(t, err)
		assert.NotErrorIs(t, err, rabbitmq.ErrMalformed)
	})
}

func ensure(t *testing.T, org *instance.Instance, ref string, revision int64, name string) amqp.Delivery {
	t.Helper()
	return amqp.Delivery{RoutingKey: rabbitmq.RoutingKeyOrgDriveEnsure, Body: mustJSON(t, map[string]any{
		"organizationId": org.OrgID,
		"reference":      map[string]any{"source": "twake-space", "id": ref},
		"revision":       revision,
		"state":          "created",
		"name":           name,
	})}
}

func onlyDrive(t *testing.T, org *instance.Instance) *sharing.Sharing {
	t.Helper()
	drives, err := sharing.ListDrives(org)
	require.NoError(t, err)
	require.Len(t, drives, 1)
	return drives[0]
}

func rootDir(t *testing.T, org *instance.Instance, s *sharing.Sharing) *vfs.DirDoc {
	t.Helper()
	dir, err := org.VFS().DirByID(s.FirstFilesRule().Values[0])
	require.NoError(t, err)
	return dir
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func newOrgInstance(t *testing.T) *instance.Instance {
	t.Helper()
	orgID := strings.ToLower("org" + utils.RandomString(10))
	setup := testutils.NewSetup(t, t.Name())
	return setup.GetTestInstance(&lifecycle.Options{
		Domain:     orgID + ".cozy.local",
		OrgID:      orgID,
		Email:      "admin@" + orgID + ".example",
		PublicName: "Organization",
	})
}
