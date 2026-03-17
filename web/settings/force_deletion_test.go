package settings_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	csettings "github.com/cozy/cozy-stack/model/settings"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	websettings "github.com/cozy/cozy-stack/web/settings"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spyRabbitMQ records published messages for assertions.
type spyRabbitMQ struct {
	mu       sync.Mutex
	messages []publishedMessage
	err      error // if set, Publish returns this error
}

type publishedMessage struct {
	Request rabbitmq.PublishRequest
}

func (s *spyRabbitMQ) StartManagers() ([]*rabbitmq.RabbitMQManager, error) {
	return nil, nil
}

func (s *spyRabbitMQ) Publish(_ context.Context, req rabbitmq.PublishRequest) error {
	if s.err != nil {
		return s.err
	}
	if req.Payload != nil {
		payload, err := json.Marshal(req.Payload)
		if err != nil {
			return err
		}
		req.Payload = payload
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, publishedMessage{
		Request: req,
	})
	return nil
}

func (s *spyRabbitMQ) last() publishedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages[len(s.messages)-1]
}

func setupForceDeletionRouter(t *testing.T, inst *instance.Instance, pdoc *permission.Permission, rmq rabbitmq.Service) *httptest.Server {
	t.Helper()

	handler := echo.New()
	handler.HTTPErrorHandler = errors.ErrorHandler
	group := handler.Group("/settings", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("instance", inst)
			if pdoc != nil {
				c.Set("permissions_doc", pdoc)
			}
			return next(c)
		}
	})

	svc := csettings.NewServiceMock(t)
	websettings.NewHTTPHandler(svc, rmq).Register(group)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func settingsAppPermission() *permission.Permission {
	return &permission.Permission{
		Type:     permission.TypeWebapp,
		SourceID: consts.Apps + "/" + consts.SettingsSlug,
	}
}

func TestForceInstanceDeletion(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	t.Run("RejectNonSettingsApp", func(t *testing.T) {
		setup := testutils.NewSetup(t, "forcedel-reject-oauth")
		inst := setup.GetTestInstance(&lifecycle.Options{Email: "alice@twake.app"})

		oauthPerm := &permission.Permission{
			Type:     permission.TypeOauth,
			SourceID: "some-oauth-client",
		}
		spy := &spyRabbitMQ{}
		ts := setupForceDeletionRouter(t, inst, oauthPerm, spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/instance/deletion/force").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(http.StatusForbidden)

		assert.Empty(t, spy.messages)
	})

	t.Run("RejectNoPermission", func(t *testing.T) {
		setup := testutils.NewSetup(t, "forcedel-reject-noperm")
		inst := setup.GetTestInstance(&lifecycle.Options{Email: "bob@twake.app"})

		spy := &spyRabbitMQ{}
		ts := setupForceDeletionRouter(t, inst, nil, spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/instance/deletion/force").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(http.StatusUnauthorized)

		assert.Empty(t, spy.messages)
	})

	t.Run("PublishDeletionMessageWithoutEmail", func(t *testing.T) {
		setup := testutils.NewSetup(t, "forcedel-noemail-ok")
		inst := setup.GetTestInstance() // no email

		spy := &spyRabbitMQ{}
		ts := setupForceDeletionRouter(t, inst, settingsAppPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/instance/deletion/force").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(http.StatusNoContent)

		require.Len(t, spy.messages, 1)
		msg := spy.last()

		var body rabbitmq.UserDeletionRequestedMessage
		require.NoError(t, json.Unmarshal(msg.Request.Payload.([]byte), &body))
		assert.Equal(t, inst.Domain, body.WorkplaceFqdn)
	})

	t.Run("ReturnErrorWhenPublishFails", func(t *testing.T) {
		setup := testutils.NewSetup(t, "forcedel-publish-fail")
		inst := setup.GetTestInstance(&lifecycle.Options{Email: "charlie@twake.app"})

		spy := &spyRabbitMQ{err: fmt.Errorf("connection refused")}
		ts := setupForceDeletionRouter(t, inst, settingsAppPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/instance/deletion/force").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(http.StatusInternalServerError)
	})

	t.Run("PublishDeletionMessage", func(t *testing.T) {
		setup := testutils.NewSetup(t, "forcedel-publish-ok")
		inst := setup.GetTestInstance(&lifecycle.Options{
			Email:       "alice@twake.app",
			ContextName: "test-ctx",
		})

		spy := &spyRabbitMQ{}
		ts := setupForceDeletionRouter(t, inst, settingsAppPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/instance/deletion/force").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(http.StatusNoContent)

		require.Len(t, spy.messages, 1)
		msg := spy.last()
		assert.Equal(t, "test-ctx", msg.Request.ContextName)
		assert.Equal(t, rabbitmq.ExchangeAuth, msg.Request.Exchange)
		assert.Equal(t, rabbitmq.RoutingKeyUserDeletionRequested, msg.Request.RoutingKey)

		var body rabbitmq.UserDeletionRequestedMessage
		require.NoError(t, json.Unmarshal(msg.Request.Payload.([]byte), &body))
		assert.Equal(t, inst.Domain, body.WorkplaceFqdn)
		assert.Equal(t, "user_request", body.Reason)
		assert.Equal(t, "cozy-stack", body.RequestedBy)
		assert.NotZero(t, body.RequestedAt)
	})

	t.Run("NoopRabbitMQReturnsError", func(t *testing.T) {
		setup := testutils.NewSetup(t, "forcedel-noop-rmq")
		inst := setup.GetTestInstance(&lifecycle.Options{Email: "alice@twake.app"})

		ts := setupForceDeletionRouter(t, inst, settingsAppPermission(), new(rabbitmq.NoopService))
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/instance/deletion/force").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(http.StatusInternalServerError).
			Body().Contains("force instance deletion requires RabbitMQ to be configured")
	})

	t.Run("PublishDeletionMessageWithRealRabbitMQ", func(t *testing.T) {
		setup := testutils.NewSetup(t, "forcedel-rmq-integration")
		inst := setup.GetTestInstance(&lifecycle.Options{ContextName: "test-ctx"})

		node := testutils.StartRabbitMQ(t, true, false)
		const queueName = "test.user.deletion.requested.settings"
		testutils.DeclareBoundQueue(t, node, rabbitmq.ExchangeAuth, queueName, rabbitmq.RoutingKeyUserDeletionRequested)

		rmq, err := rabbitmq.NewService(config.RabbitMQ{
			Enabled: true,
			Nodes: map[string]config.RabbitMQNode{
				"test-ctx": {
					Enabled: true,
					URL:     node.AMQPURL,
				},
			},
		})
		require.NoError(t, err)

		ts := setupForceDeletionRouter(t, inst, settingsAppPermission(), rmq)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/instance/deletion/force").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(http.StatusNoContent)

		msg, ok := testutils.GetOneFromQueue(t, node, queueName, 5*time.Second)
		require.True(t, ok, "expected a published message in %s", queueName)
		assert.Equal(t, "application/json", msg.ContentType)

		var body rabbitmq.UserDeletionRequestedMessage
		require.NoError(t, json.Unmarshal(msg.Body, &body))
		assert.Equal(t, inst.Domain, body.WorkplaceFqdn)
		assert.Equal(t, "user_request", body.Reason)
		assert.Equal(t, "cozy-stack", body.RequestedBy)
		assert.NotZero(t, body.RequestedAt)
	})
}
