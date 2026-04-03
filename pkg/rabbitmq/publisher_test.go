package rabbitmq

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestWaitForPublishResultIgnoresClosedReturnChannel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	confirms := make(chan amqp.Confirmation, 1)
	confirms <- amqp.Confirmation{Ack: true}

	returns := make(chan amqp.Return)
	close(returns)

	err := waitForPublishResult(ctx, ExchangeAuth, RoutingKeyUserDeletionRequested, confirms, returns)
	require.NoError(t, err)
}

func TestValidateAcceptsRawBodyWithoutPayload(t *testing.T) {
	t.Parallel()
	req := PublishRequest{
		Exchange:   "test.exchange",
		RoutingKey: "test.key",
		RawBody:    []byte("binary data"),
	}
	require.NoError(t, req.validate())
}

func TestValidateRejectsNoPayloadAndNoRawBody(t *testing.T) {
	t.Parallel()
	req := PublishRequest{
		Exchange:   "test.exchange",
		RoutingKey: "test.key",
	}
	require.Error(t, req.validate())
}

func TestMarshalPayloadReturnsRawBodyWhenSet(t *testing.T) {
	t.Parallel()
	raw := []byte("binary content here")
	req := PublishRequest{
		Exchange:   "test.exchange",
		RoutingKey: "test.key",
		RawBody:    raw,
	}
	body, err := req.marshalPayload()
	require.NoError(t, err)
	require.Equal(t, raw, body)
}

func TestMarshalPayloadFallsBackToJSONWhenNoRawBody(t *testing.T) {
	t.Parallel()
	req := PublishRequest{
		Exchange:   "test.exchange",
		RoutingKey: "test.key",
		Payload:    map[string]string{"key": "value"},
	}
	body, err := req.marshalPayload()
	require.NoError(t, err)
	require.JSONEq(t, `{"key":"value"}`, string(body))
}

func TestToAMQPPublishingUsesCustomContentTypeAndHeaders(t *testing.T) {
	t.Parallel()
	headers := amqp.Table{"action": "upsert", "partition": "test.domain"}
	req := PublishRequest{
		Exchange:    "test.exchange",
		RoutingKey:  "test.key",
		RawBody:     []byte("file bytes"),
		ContentType: "application/octet-stream",
		Headers:     headers,
	}
	body, _ := req.marshalPayload()
	pub := req.toAMQPPublishing(body)

	require.Equal(t, "application/octet-stream", pub.ContentType)
	require.Equal(t, amqp.Table{"action": "upsert", "partition": "test.domain"}, pub.Headers)
	require.Equal(t, []byte("file bytes"), pub.Body)
}

func TestToAMQPPublishingDefaultsToJSONContentType(t *testing.T) {
	t.Parallel()
	req := PublishRequest{
		Exchange:   "test.exchange",
		RoutingKey: "test.key",
		Payload:    "test",
	}
	body, _ := req.marshalPayload()
	pub := req.toAMQPPublishing(body)

	require.Equal(t, "application/json", pub.ContentType)
	require.Nil(t, pub.Headers)
}

func TestWaitForPublishResultPrefersReturnedMessageWhenConfirmChannelCloses(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	confirms := make(chan amqp.Confirmation)
	close(confirms)

	returns := make(chan amqp.Return, 1)
	returns <- amqp.Return{ReplyCode: 312, ReplyText: "NO_ROUTE"}

	err := waitForPublishResult(ctx, ExchangeAuth, RoutingKeyUserDeletionRequested, confirms, returns)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPublishReturned)

	var returned *PublishReturnedError
	require.True(t, errors.As(err, &returned))
	require.Equal(t, uint16(312), returned.ReplyCode)
	require.Equal(t, "NO_ROUTE", returned.ReplyText)
}
