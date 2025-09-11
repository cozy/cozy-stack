package rabbitmq_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/tests/testutils"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestRabbitMQManager(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped with --short")
	}
	t.Run("ConsumeAll_NoLoss", func(t *testing.T) {
		t.Parallel()

		// Start broker
		rabbitmq := testutils.StartRabbitMQ(t, true, false)
		totalMessages := 10

		// Build manager with one exchange and one queue using PasswordChangeHandler wrapper
		handler := newCountingTestHandler()
		mgr := initRabbitMQManager(rabbitmq.AMQPURL, "auth", "user.password.updated", "password.changed", handler)
		_, err := mgr.Start(testCtx(t))
		require.NoError(t, err)

		// Wait for manager to declare topology
		require.NoError(t, mgr.WaitReady(testCtx(t)))

		sender := newTestMessageSender(t, rabbitmq.AMQPURL, "auth", "password.changed")
		// Publish all messages
		for i := 0; i < totalMessages; i++ {
			msg := testMessage{TimeStamp: time.Now().UnixMilli()}
			sender.publish(msg)
		}

		// Await full consumption
		testutils.WaitForOrFail(t, 10*time.Second, func() bool {
			return handler.total >= totalMessages
		})
	})

	t.Run("ConsumeAll_WithReconnection", func(t *testing.T) {
		t.Parallel()

		// Start broker
		MQ := testutils.StartRabbitMQ(t, true, false)
		totalMessages := 20

		// Build manager with one exchange and one queue using PasswordChangeHandler wrapper
		handler := newCountingTestHandler()
		mgr := initRabbitMQManager(MQ.AMQPURL, "auth", "user.password.updated", "password.changed", handler)
		_, err := mgr.Start(testCtx(t))
		require.NoError(t, err)

		require.NoError(t, mgr.WaitReady(testCtx(t)))

		sender := newTestMessageSender(t, MQ.AMQPURL, "auth", "password.changed")
		// Publish half of the messages
		for i := 0; i < totalMessages/2; i++ {
			sender.publish(testMessage{TimeStamp: time.Now().UnixMilli()})
		}

		// restart the container
		MQ.Stop(context.Background(), 30*time.Second)
		MQ.Restart(context.Background(), 30*time.Second)

		// publish second half
		sender = newTestMessageSender(t, MQ.AMQPURL, "auth", "password.changed")
		// Publish half of the messages
		for i := 0; i < totalMessages/2; i++ {
			sender.publish(testMessage{TimeStamp: time.Now().UnixMilli()})
		}

		// Await full consumption
		testutils.WaitForOrFail(t, 10*time.Second, func() bool {
			return handler.total >= totalMessages
		})
		require.NoError(t, mgr.Shutdown(testCtx(t)))
	})
}

func TestPasswordHandler(t *testing.T) {
	// Start RabbitMQ broker
	MQ := testutils.StartRabbitMQ(t, false, false)
	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	if testing.Short() {
		t.Skip("integration test skipped with --short")
	}

	t.Run("ChangePasswordWithKeys", func(t *testing.T) {
		// Configure RabbitMQ before starting the stack so it is initialized by the stack
		setup := setUpRabbitMQConfig(t, MQ, "ChangePasswordWithoutKeys")
		inst := setup.GetTestInstance()

		domain := inst.Domain

		// Publisher conn/channel
		err, ch := getChannel(t, MQ)

		// Compose message
		testHash := "testhash123"
		msg := rabbitmq.PasswordChangeMessage{
			TwakeID:    inst.Prefix,
			Iterations: 100000,
			Hash:       testHash,
			PublicKey:  "PUB",
			PrivateKey: "PRIV",
			Key:        "KEY",
			Timestamp:  time.Now().Unix(),
			Domain:     domain,
		}
		body, err := json.Marshal(msg)
		require.NoError(t, err)

		// Publish
		err = ch.PublishWithContext(
			testCtx(t),
			"auth",
			"password.changed",
			false,
			false,
			amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				ContentType:  "application/json",
				Body:         body,
				MessageId:    fmt.Sprintf("%d", time.Now().UnixNano()),
			},
		)
		require.NoError(t, err)

		// Wait until the instance hash is updated
		testutils.WaitForOrFail(t, 10*time.Second, func() bool {
			updated, err := lifecycle.GetInstance(domain)
			return err == nil && string(updated.PassphraseHash) == testHash
		})
	})

	t.Run("ChangePasswordWithoutKeys", func(t *testing.T) {
		// Configure RabbitMQ before starting the stack so it is initialized by the stack
		setup := setUpRabbitMQConfig(t, MQ, "ChangePasswordWithoutKeys")
		inst := setup.GetTestInstance()

		domain := inst.Domain

		// Publisher conn/channel
		err, ch := getChannel(t, MQ)

		// Compose message
		testHash := "testhash1234"
		msg := rabbitmq.PasswordChangeMessage{
			TwakeID:    inst.Prefix,
			Iterations: 100000,
			Hash:       testHash,
			Domain:     domain,
		}
		body, err := json.Marshal(msg)
		require.NoError(t, err)

		// Publish
		err = ch.PublishWithContext(
			testCtx(t),
			"auth",
			"password.changed",
			false,
			false,
			amqp.Publishing{
				DeliveryMode: amqp.Persistent,
				ContentType:  "application/json",
				Body:         body,
				MessageId:    fmt.Sprintf("%d", time.Now().UnixNano()),
			},
		)
		require.NoError(t, err)

		// Wait until the instance hash is updated
		testutils.WaitForOrFail(t, 30*time.Second, func() bool {
			updated, err := lifecycle.GetInstance(domain)
			return err == nil && string(updated.PassphraseHash) == testHash
		})
	})
}

func getChannel(t *testing.T, mq *testutils.RabbitFixture) (error, *amqp.Channel) {
	t.Helper()
	conn, err := amqp.Dial(mq.AMQPURL)
	require.NoError(t, err)
	ch, err := conn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ch.Close(); _ = conn.Close() })
	return err, ch
}

func setUpRabbitMQConfig(t *testing.T, mq *testutils.RabbitFixture, name string) *testutils.TestSetup {
	cfg := config.GetConfig()

	cfg.RabbitMQ.Enabled = true
	cfg.RabbitMQ.URL = mq.AMQPURL
	cfg.RabbitMQ.Exchanges = []config.RabbitExchange{
		{
			Name:            "auth",
			Kind:            "topic",
			Durable:         true,
			DeclareExchange: true,
			Queues: []config.RabbitQueue{
				{
					Name:     "user.password.updated",
					Bindings: []string{"password.changed"},
					Prefetch: 4,
					Declare:  true,
				},
			},
		},
	}

	// Start the stack via testutils and create an instance
	setup := testutils.NewSetup(t, name)
	return setup
}

func initConnection(t *testing.T, mgr *rabbitmq.RabbitMQConnection) *amqp.Connection {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := mgr.Connect(ctx, 5)
	require.NoError(t, err)
	require.NotNil(t, conn)
	return conn
}

func declareTestQueue(t *testing.T, conn *amqp.Connection, name string) {
	t.Helper()
	ch, err := conn.Channel()
	require.NoError(t, err)
	_, err = ch.QueueDeclare(name, false, true, false, false, nil)
	require.NoError(t, err)
	err = ch.Close()
	require.NoError(t, err)
}

func TestConnection(t *testing.T) {
	t.Parallel()

	t.Run("InitConnectionAndQueue", func(t *testing.T) {
		t.Parallel()

		f := testutils.StartRabbitMQ(t, false, false)

		connMgr := rabbitmq.NewRabbitMQConnection(f.AMQPURL)
		conn := initConnection(t, connMgr)

		// queue declaration should work find
		declareTestQueue(t, conn, "testcontainers-queue")
		require.NoError(t, connMgr.Close(), "can")
	})

	t.Run("MonitorClose", func(t *testing.T) {
		t.Parallel()

		MQ := testutils.StartRabbitMQ(t, false, false)

		mgr := rabbitmq.NewRabbitMQConnection(MQ.AMQPURL)

		_ = initConnection(t, mgr)

		monitor := mgr.MonitorConnection()
		require.NotNil(t, monitor)

		// stop container
		MQ.Stop(context.Background(), 30*time.Second)

		// asserts a close notification is received.
		select {
		case <-time.After(30 * time.Second):
			t.Fatalf("did not receive connection close notification within timeout")
		case err := <-monitor:
			require.Error(t, err)
		}
	})

	t.Run("ReconnectAfterDowntime", func(t *testing.T) {
		t.Parallel()

		MQ := testutils.StartRabbitMQ(t, false, false)

		conn := rabbitmq.NewRabbitMQConnection(MQ.AMQPURL)

		// Initial connection
		_ = initConnection(t, conn)

		// Stop broker
		MQ.Stop(context.Background(), 30*time.Second)

		// Restart container,update manager, reconnect
		MQ.Restart(context.Background(), 30*time.Second)

		// update manager, reconnect
		rconn, err := conn.Connect(context.Background(), 10)
		require.NoError(t, err)
		require.NotNil(t, rconn)
	})

	t.Run("ConnectWithTLS", func(t *testing.T) {
		t.Parallel()

		f := testutils.StartRabbitMQ(t, false, true)

		cm := rabbitmq.NewRabbitMQConnection(f.AMQPURL)
		cm.TLSConfig = getTlsConfig(t)

		conn := initConnection(t, cm)

		declareTestQueue(t, conn, "tls-test-queue")
		require.NoError(t, cm.Close())
	})

	t.Run("ConnectWithTLSIgnoreCA", func(t *testing.T) {
		t.Parallel()

		f := testutils.StartRabbitMQ(t, false, true)

		cm := rabbitmq.NewRabbitMQConnection(f.AMQPURL)
		cm.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}

		conn := initConnection(t, cm)

		declareTestQueue(t, conn, "tls-test-queue")
		require.NoError(t, cm.Close())
	})
}

func certRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(thisFile)
	return filepath.Join(testDir, "testdata")
}

func getTlsConfig(t *testing.T) *tls.Config {
	t.Helper()
	caPEM, err := os.ReadFile(filepath.Join(certRoot(), "ca.pem"))
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(caPEM))
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
	return tlsCfg
}

type countingTestHandler struct {
	total int
}

func newCountingTestHandler() *countingTestHandler {
	return &countingTestHandler{
		total: 0,
	}
}

func (h *countingTestHandler) Handle(ctx context.Context, d amqp.Delivery) error {
	h.total++
	return nil
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func initRabbitMQManager(amqpURL string, exchangeName string, queueName string, routingKey string, handler *countingTestHandler) *rabbitmq.RabbitMQManager {
	exchangeCfg := &config.RabbitExchange{
		Name:            exchangeName,
		Kind:            "topic",
		DeclareExchange: true,
		Durable:         true,
	}
	queueCfg := &config.RabbitQueue{
		Name:          queueName,
		Bindings:      []string{routingKey},
		Declare:       true,
		Prefetch:      8,
		DeliveryLimit: 5,
	}

	exchange := rabbitmq.NewExchangeSpec(exchangeCfg)
	queue := rabbitmq.NewQueueSpec(queueCfg, handler, "", "")
	exchange.Queues = []rabbitmq.QueueSpec{queue}

	exchanges := []rabbitmq.ExchangeSpec{exchange}
	return rabbitmq.NewRabbitMQManager(amqpURL, exchanges)
}

type testMessage struct {
	TimeStamp int64
}

type testMessageSender struct {
	t            *testing.T
	ch           *amqp.Channel
	exchangeName string
	routingKey   string
}

func newTestMessageSender(t *testing.T, amqpURL string, exchangeName string, routingKey string) *testMessageSender {
	conn := rabbitmq.NewRabbitMQConnection(amqpURL)
	pubConn, err := conn.Connect(context.Background(), 5)
	require.NoError(t, err)
	pubCh, err := pubConn.Channel()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pubCh.Close() })
	return &testMessageSender{
		t:            t,
		ch:           pubCh,
		exchangeName: exchangeName,
		routingKey:   routingKey,
	}
}

func (m *testMessageSender) publish(msg testMessage) {
	m.t.Helper()
	body, err := json.Marshal(msg)
	require.NoError(m.t, err)
	require.NoError(m.t, m.ch.PublishWithContext(
		context.Background(),
		m.exchangeName,
		m.routingKey,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
		},
	))
}
