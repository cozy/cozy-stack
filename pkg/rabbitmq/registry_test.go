package rabbitmq_test

import (
	"context"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type registeredHandler struct {
	publisher rabbitmq.Publisher
}

func (h *registeredHandler) Handle(context.Context, amqp.Delivery) error { return nil }

type nopPublisher struct{}

func (nopPublisher) Publish(context.Context, rabbitmq.PublishRequest) error { return nil }

func TestRegisteredHandlerGetsThePublisher(t *testing.T) {
	var built *registeredHandler
	rabbitmq.RegisterHandler("test.registered", func(p rabbitmq.Publisher) rabbitmq.Handler {
		built = &registeredHandler{publisher: p}
		return built
	})

	pub := nopPublisher{}
	specs := rabbitmq.BuildExchangeSpecs([]config.RabbitExchange{{
		Name:   "test",
		Kind:   "topic",
		Queues: []config.RabbitQueue{{Name: "test.registered"}, {Name: "test.unknown"}},
	}}, pub)

	require.Len(t, specs, 1)
	require.Len(t, specs[0].Queues, 1, "only the registered queue gets a handler")
	assert.Same(t, built, specs[0].Queues[0].Handler)
	assert.Equal(t, pub, built.publisher)
}

func TestSetupWorkersGivesThePublisher(t *testing.T) {
	var got rabbitmq.Publisher
	rabbitmq.RegisterWorkers(func(p rabbitmq.Publisher) { got = p })

	pub := nopPublisher{}
	rabbitmq.SetupWorkers(pub)

	assert.Equal(t, pub, got)
}
