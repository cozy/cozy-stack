package rabbitmq_test

import (
	"context"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceImplem(t *testing.T) {
	assert.Implements(t, (*rabbitmq.Service)(nil), new(rabbitmq.RabbitMQService))
}

func TestRabbitMQService(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped with --short")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, "RabbitMQService")
	inst := setup.GetTestInstance()

	defaultNode := testutils.StartRabbitMQ(t, true, false)
	twakeNode := testutils.StartRabbitMQ(t, true, false)

	cfg := config.RabbitMQ{
		Enabled: true,
		Nodes: map[string]config.RabbitMQNode{
			"default": {
				Enabled: true,
				URL:     defaultNode.AMQPURL,
			},
			"linagora_default": {
				Enabled: false,
				URL:     "amqp://linagora:password@localhost",
			},
			"twake_default": {
				Enabled: true,
				URL:     twakeNode.AMQPURL,
			},
		},
		Exchanges: []config.RabbitExchange{
			{
				Name:            "auth",
				Kind:            "topic",
				Durable:         false,
				DeclareExchange: true,
				Queues: []config.RabbitQueue{
					{
						Name:     "stack.user.password.updated",
						Bindings: []string{"user.password.updated"},
						Declare:  true,
					},
				},
			},
		},
	}

	t.Run("NewServiceBuildsManagerForEachNode", func(t *testing.T) {
		t.Parallel()

		svc, err := rabbitmq.NewService(cfg)
		require.NoError(t, err)

		require.Len(t, svc.Managers, 2)
		assert.NotNil(t, svc.Managers["default"])
		assert.NotNil(t, svc.Managers["twake_default"])
	})

	t.Run("StartManagersStartsEachManager", func(t *testing.T) {
		t.Parallel()

		svc, err := rabbitmq.NewService(cfg)
		require.NoError(t, err)

		managers, err := svc.StartManagers()
		require.NoError(t, err)
		require.Len(t, managers, 2)

		err = managers[0].WaitReady(context.Background())
		assert.Nil(t, err, "RabbitMQ manager for context default failed to start")

		err = managers[1].WaitReady(context.Background())
		assert.Nil(t, err, "RabbitMQ manager for context twake_default failed to start")
	})

	t.Run("RabbitMQManagersReceiveMessages", func(t *testing.T) {
		// TODO: find a way to make sure each message is handled only by the
		// appropriate manager.
		t.Parallel()

		svc, err := rabbitmq.NewService(cfg)
		require.NoError(t, err)

		_, err = svc.StartManagers()
		require.NoError(t, err)

		slug, domain := inst.SlugAndDomain()

		// 1. Update instance password via default RabbitMQ node
		defaultSender := newTestMessageSender(t, defaultNode.AMQPURL, "auth", "user.password.updated")
		hashText, hashB64 := hashPassphrase(t)
		msg := rabbitmq.PasswordChangeMessage{
			TwakeID:       slug,
			Iterations:    100000,
			Hash:          hashB64,
			PublicKey:     "PUB",
			Key:           "KEY",
			Timestamp:     time.Now().Unix(),
			WorkplaceFqdn: slug + "." + domain,
		}
		defaultSender.publish(msg)

		// Wait until the instance hash is updated
		testutils.WaitForOrFail(t, 10*time.Second, func() bool {
			updated, err := lifecycle.GetInstance(inst.Domain)
			return err == nil && string(updated.PassphraseHash) == hashText
		})

		// 2. Update instance password via twake RabbitMQ node
		twakeSender := newTestMessageSender(t, twakeNode.AMQPURL, "auth", "user.password.updated")
		hashText, hashB64 = hashPassphrase(t)
		msg = rabbitmq.PasswordChangeMessage{
			TwakeID:       slug,
			Iterations:    100000,
			Hash:          hashB64,
			PublicKey:     "PUB",
			Key:           "KEY",
			Timestamp:     time.Now().Unix(),
			WorkplaceFqdn: slug + "." + domain,
		}
		twakeSender.publish(msg)

		// Wait until the instance hash is updated
		testutils.WaitForOrFail(t, 10*time.Second, func() bool {
			updated, err := lifecycle.GetInstance(inst.Domain)
			return err == nil && string(updated.PassphraseHash) == hashText
		})
	})
}
