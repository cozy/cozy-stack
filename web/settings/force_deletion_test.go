package settings_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

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
	ContextName string
	Exchange    string
	RoutingKey  string
	Body        []byte
}

func (s *spyRabbitMQ) StartManagers() ([]*rabbitmq.RabbitMQManager, error) {
	return nil, nil
}

func (s *spyRabbitMQ) Publish(_ context.Context, contextName, exchange, routingKey string, body []byte) error {
	if s.err != nil {
		return s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, publishedMessage{
		ContextName: contextName,
		Exchange:    exchange,
		RoutingKey:  routingKey,
		Body:        body,
	})
	return nil
}

func (s *spyRabbitMQ) ClosePublishers(_ context.Context) error {
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

	t.Run("RejectMissingEmail", func(t *testing.T) {
		setup := testutils.NewSetup(t, "forcedel-reject-noemail")
		inst := setup.GetTestInstance() // no email

		spy := &spyRabbitMQ{}
		ts := setupForceDeletionRouter(t, inst, settingsAppPermission(), spy)
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/instance/deletion/force").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(http.StatusBadRequest)

		assert.Empty(t, spy.messages)
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
		assert.Equal(t, "test-ctx", msg.ContextName)
		assert.Equal(t, "auth", msg.Exchange)
		assert.Equal(t, "user.deletion.requested", msg.RoutingKey)

		var body map[string]any
		require.NoError(t, json.Unmarshal(msg.Body, &body))
		assert.Equal(t, "alice@twake.app", body["email"])
		assert.Equal(t, "user_request", body["reason"])
		assert.Equal(t, "cozy-stack", body["requestedBy"])
		assert.NotZero(t, body["requestedAt"])
	})

	t.Run("NoopRabbitMQReturnsError", func(t *testing.T) {
		setup := testutils.NewSetup(t, "forcedel-noop-rmq")
		inst := setup.GetTestInstance(&lifecycle.Options{Email: "alice@twake.app"})

		ts := setupForceDeletionRouter(t, inst, settingsAppPermission(), new(rabbitmq.NoopService))
		e := testutils.CreateTestClient(t, ts.URL)

		e.POST("/settings/instance/deletion/force").
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(http.StatusInternalServerError)
	})
}
