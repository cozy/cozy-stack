package rabbitmq

import (
	"context"
	"fmt"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startRabbitMQContainer(t *testing.T) (tc.Container, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	username := "guest"
	password := "guest"

	req := tc.ContainerRequest{
		Image:        "rabbitmq:3.13-management",
		ExposedPorts: []string{"5672/tcp"},
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": username,
			"RABBITMQ_DEFAULT_PASS": password,
		},
		WaitingFor: wait.ForListeningPort("5672/tcp").WithStartupTimeout(1 * time.Minute),
	}

	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: req, Started: true})
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)
	mapped, err := container.MappedPort(ctx, "5672/tcp")
	require.NoError(t, err)

	amqpURL := fmt.Sprintf("amqp://%s:%s@%s:%s/", username, password, host, mapped.Port())
	return container, amqpURL
}

func initConnection(t *testing.T, mgr *RabbitMQConnection) *amqp.Connection {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	_ = ch.Close()
	require.NoError(t, err)
}

func stopAndExpectClose(t *testing.T, container tc.Container, mgr *RabbitMQConnection) {
	t.Helper()
	stopTimeout := 30 * time.Second
	waitForClose := 30 * time.Second

	monitor := mgr.MonitorConnection()
	require.NotNil(t, monitor)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), stopTimeout)
	defer stopCancel()
	require.NoError(t, container.Stop(stopCtx, &stopTimeout))

	//asserts a close notification is received.
	select {
	case <-time.After(waitForClose):
		t.Fatalf("did not receive connection close notification within timeout")
	case err := <-monitor:
		require.Error(t, err)
	}
}

// waitForBroker waits until the broker accepts AMQP connections
func waitForBroker(t *testing.T, amqpURL string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("rabbitmq did not become ready within %s", timeout)
		}
		conn, err := amqp.Dial(amqpURL)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// rebuildAMQPURL rebuilds the AMQP URL from the container's current mapped port.
func rebuildAMQPURL(t *testing.T, container tc.Container, ctx context.Context) string {
	t.Helper()
	host, err := container.Host(ctx)
	require.NoError(t, err)
	mapped, err := container.MappedPort(ctx, "5672/tcp")
	require.NoError(t, err)
	return fmt.Sprintf("amqp://%s:%s@%s:%s/", "guest", "guest", host, mapped.Port())
}

func TestConnectionTest(t *testing.T) {
	t.Run("InitConnectionAndQueue", func(t *testing.T) {
		t.Parallel()

		container, amqpURL := startRabbitMQContainer(t)
		t.Cleanup(func() { _ = container.Terminate(context.Background()) })

		connMgr := NewRabbitMQConnection(amqpURL)

		conn := initConnection(t, connMgr)

		//queue declaration should work find
		declareTestQueue(t, conn, "testcontainers-queue")
	})

	t.Run("MonitorClose", func(t *testing.T) {
		t.Parallel()

		container, amqpURL := startRabbitMQContainer(t)
		t.Cleanup(func() { _ = container.Terminate(context.Background()) })

		connMgr := NewRabbitMQConnection(amqpURL)

		_ = initConnection(t, connMgr)
		stopAndExpectClose(t, container, connMgr)
	})

	t.Run("ReconnectAfterDowntime", func(t *testing.T) {
		t.Parallel()

		container, amqpURL := startRabbitMQContainer(t)
		t.Cleanup(func() { _ = container.Terminate(context.Background()) })

		connMgr := NewRabbitMQConnection(amqpURL)

		// Initial connection
		_ = initConnection(t, connMgr)

		// Stop broker
		stopAndExpectClose(t, container, connMgr)

		// Restart container,update manager, reconnect
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()
		require.NoError(t, container.Start(ctx))

		// rebuild URL
		newURL := rebuildAMQPURL(t, container, ctx)
		waitForBroker(t, newURL, 1*time.Minute)

		//update manager, reconnect
		connMgr.url = newURL
		reconnectCtx, reconnectCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer reconnectCancel()
		conn, err := connMgr.Connect(reconnectCtx, 10)
		require.NoError(t, err)
		require.NotNil(t, conn)
	})
}
