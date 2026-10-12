package rabbitmq_test

import (
	"context"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/messaging"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type registeredHandler struct {
	publisher messaging.Publisher
}

func (h *registeredHandler) Handle(context.Context, amqp.Delivery) error { return nil }

func TestRegisteredHandlerGetsThePublisher(t *testing.T) {
	var built *registeredHandler
	rabbitmq.RegisterHandler("test.registered", func(p messaging.Publisher) rabbitmq.Handler {
		built = &registeredHandler{publisher: p}
		return built
	})

	svc := new(rabbitmq.RabbitMQService)
	specs := svc.BuildExchangeSpecs([]config.RabbitExchange{{
		Name:   "test",
		Kind:   "topic",
		Queues: []config.RabbitQueue{{Name: "test.registered"}, {Name: "test.unknown"}},
	}})

	require.Len(t, specs, 1)
	require.Len(t, specs[0].Queues, 1, "only the registered queue gets a handler")
	assert.Same(t, built, specs[0].Queues[0].Handler)
	assert.Same(t, svc, built.publisher)
}
