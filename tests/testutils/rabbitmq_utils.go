package testutils

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/rabbitmq"
	"github.com/docker/go-connections/nat"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"

	c "github.com/docker/docker/api/types/container"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type RabbitFixture struct {
	Container   tc.Container
	AMQPURL     string // e.g. amqp://user:pass@127.0.0.1:randomPort/
	AMQPSURL    string // e.g. amqps://user:pass@127.0.0.1:randomPort/
	ManageURL   string // e.g. http://user:pass@127.0.0.1:randomPort/
	Username    string
	Password    string
	MappedAMQP  string
	MappedAMQPS string
	MappedHTTP  string
	enableTLS   bool
	t           *testing.T
}

// StartRabbitMQ starts up a RabbitMQ container with random host ports
func StartRabbitMQ(t *testing.T, withVolume bool, enableTLS bool) *RabbitFixture {
	t.Helper()

	user := "guest"
	pass := "guest"

	// Unique volume name if you want persistence across Stop/Start *within the test*.
	amqpHostPort := getFreePort(t)
	httpHostPort := getFreePort(t)
	amqpsHostPort := ""

	volName := "rmq_" + regexp.MustCompile(`[^a-z0-9_.-]+`).ReplaceAllString(strings.ToLower(t.Name()), "")
	hostCfg := func(hc *c.HostConfig) {
		hc.PortBindings = nat.PortMap{
			"5672/tcp":  []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: amqpHostPort}},
			"15672/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: httpHostPort}},
		}
		if enableTLS {
			amqpsHostPort = getFreePort(t)
			hc.PortBindings["5671/tcp"] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: amqpsHostPort}}

			hc.Binds = append(hc.Binds,
				fmt.Sprintf("%s:/etc/rabbitmq/rabbitmq.conf", filepath.Join(certRoot(), "rabbitmq.conf")),
				fmt.Sprintf("%s:/etc/rabbitmq/certs", certRoot()),
			)
		}
		if withVolume {
			hc.Binds = append(hc.Binds, fmt.Sprintf("%s:/var/lib/rabbitmq/mnesia", volName))
		}
	}

	req := tc.ContainerRequest{
		Image:        "rabbitmq:3.13-management",
		ExposedPorts: []string{"5672/tcp", "15672/tcp", "5671/tcp"},
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": user,
			"RABBITMQ_DEFAULT_PASS": pass,
		},
		HostConfigModifier: hostCfg,
		// Wait for AMQP, mgmt API,
		WaitingFor: func() wait.Strategy {
			base := []wait.Strategy{
				wait.ForListeningPort("5672/tcp"),
				wait.ForHTTP("/api/overview").
					WithPort("15672/tcp").
					WithBasicAuth(user, pass).
					WithStartupTimeout(60 * time.Second),
			}
			return wait.ForAll(base...)
		}(),
	}

	container, err := tc.GenericContainer(context.Background(), tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start RabbitMQ")

	host, err := container.Host(context.Background())
	require.NoError(t, err, "failed to get host for RabbitMQ")

	fixture := &RabbitFixture{
		Container:  container,
		Username:   user,
		Password:   pass,
		MappedAMQP: fmt.Sprintf("%s:%s", host, amqpHostPort),
		MappedHTTP: fmt.Sprintf("%s:%s", host, httpHostPort),
		AMQPURL:    fmt.Sprintf("amqp://%s:%s@%s:%s/", user, pass, host, amqpHostPort),
		ManageURL:  fmt.Sprintf("http://%s:%s@%s:%s/", user, pass, host, httpHostPort),
		enableTLS:  enableTLS,
		t:          t,
	}
	if enableTLS {
		fixture.MappedAMQPS = fmt.Sprintf("%s:%s", host, amqpsHostPort)
		fixture.AMQPSURL = fmt.Sprintf("amqps://%s:%s@%s:%s/", user, pass, host, amqpsHostPort)
	}

	fixture.t.Logf("AMQP: %s", fixture.AMQPURL)
	fixture.t.Logf("Mgmt: %s", fixture.ManageURL)
	if enableTLS {
		fixture.t.Logf("AMQPS: %s", fixture.AMQPSURL)
	}

	t.Cleanup(func() {
		_ = container.Terminate(context.Background())
		_ = exec.Command("docker", "volume", "rm", volName).Run()
	})

	return fixture
}

func (f *RabbitFixture) Publish() {
	conn, err := amqp.Dial(f.AMQPURL)
	require.NoError(f.t, err)
	ch, err := conn.Channel()
	require.NoError(f.t, err)
	f.t.Cleanup(func() { _ = ch.Close(); _ = conn.Close() })

	// Compose message
	testHash := "testhash123"
	domain := "test.example.com"
	msg := rabbitmq.PasswordChangeMessage{
		TwakeID:    "user-123",
		Iterations: 100000,
		Hash:       testHash,
		PublicKey:  "PUB",
		PrivateKey: "PRIV",
		Key:        "KEY",
		Timestamp:  time.Now().Unix(),
		Domain:     domain,
	}
	_, err = json.Marshal(msg)
	require.NoError(f.t, err)
}

func (f *RabbitFixture) Restart(ctx context.Context, timeout time.Duration) {
	f.t.Helper()

	err := f.Container.Start(ctx)
	require.NoError(f.t, err, "failed to start RabbitMQ container")

	// Wait until container reports Running
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := f.Container.State(ctx)
		require.NoError(f.t, err, "error checking container state")
		if state.Running {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Now wait for RabbitMQ services to be actually ready
	waitStrategy := wait.ForAll(
		wait.ForListeningPort("5672/tcp"),
		wait.ForHTTP("/api/overview").
			WithPort("15672/tcp").
			WithBasicAuth(f.Username, f.Password).
			WithStartupTimeout(timeout),
	)

	err = waitStrategy.WaitUntilReady(ctx, f.Container)
	require.NoError(f.t, err, "container did not become ready")

	// Refresh URLs in case Docker reassigns (rare, but cheap check)
	host, err := f.Container.Host(ctx)
	require.NoError(f.t, err, "can't get a host of a container")
	amqpPort, err := f.Container.MappedPort(ctx, "5672/tcp")
	require.NoError(f.t, err, "can't get an amqp port of a container")
	httpPort, err := f.Container.MappedPort(ctx, "15672/tcp")
	require.NoError(f.t, err, "can't get a http port of a container")

	f.AMQPURL = fmt.Sprintf("amqp://%s:%s@%s:%s/", f.Username, f.Password, host, amqpPort.Port())
	f.ManageURL = fmt.Sprintf("http://%s:%s@%s:%s/", f.Username, f.Password, host, httpPort.Port())

	if f.enableTLS {
		ampqsPort, err := f.Container.MappedPort(ctx, "5671/tcp")
		f.AMQPSURL = fmt.Sprintf("amqps://%s:%s@%s:%s/", f.Username, f.Password, host, ampqsPort.Port())
		require.NoError(f.t, err, "can't get a ampqs port of a container")
	}
	f.t.Logf("AMQP: %s", f.AMQPURL)
	f.t.Logf("Mgmt: %s", f.ManageURL)
	if f.AMQPSURL != "" {
		f.t.Logf("AMQPS: %s", f.AMQPSURL)
	}
}

func (f *RabbitFixture) Stop(ctx context.Context, timeout time.Duration) {
	err := f.Container.Stop(ctx, &timeout)
	require.NoError(f.t, err)

	// Poll the container state until it's not running
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := f.Container.State(ctx)
		require.NoError(f.t, err, fmt.Errorf("error checking container state: %w", err))
		if !state.Running {
			return // fully stopped
		}
		time.Sleep(100 * time.Millisecond)
	}
	f.t.Errorf("container did not stop within %s", timeout)
}

func getFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	_, p, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)
	return p
}

func certRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(thisFile)
	return filepath.Join(testDir, "testdata")
}
