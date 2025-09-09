package rabbitmq

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
)

func TestBuildExchangeSpecs_ResolutionAndValidation(t *testing.T) {
	cfg := config.RabbitMQ{
		Enabled: true,
		URL:     "amqp://guest:guest@localhost:5672/",
		Exchanges: []config.RabbitExchange{
			{
				Name:    "valid-exchange",
				Kind:    "topic",
				Durable: true,
				DLXName: "ex-dlx",
				DLQName: "q-dlq",
				Queues: []config.RabbitQueue{
					{
						Name:          "q1",
						Bindings:      []string{"rk.1"},
						Prefetch:      0,
						DeliveryLimit: 5,
						// no overrides → use exchange defaults
					},
					{
						Name:          "q2",
						Bindings:      []string{"rk.2"},
						Prefetch:      10,
						DeliveryLimit: 10,
						DLXName:       "q2-dlx",
						DLQName:       "q2-dlq",
					},
					{
						Name:     "", // invalid, should be skipped
						Prefetch: 10,
					},
				},
			},
			{
				Name: "", // invalid exchange, should be skipped
				Kind: "topic",
			},
		},
	}

	specs := buildExchangeSpecs(cfg)
	if len(specs) != 1 {
		t.Fatalf("expected 1 valid exchange, got %d", len(specs))
	}

	ex := specs[0]
	if ex.cfg.Name != "valid-exchange" {
		t.Fatalf("unexpected exchange name: %s", ex.cfg.Name)
	}

	if len(ex.Queues) != 2 {
		t.Fatalf("expected 2 valid queues, got %d", len(ex.Queues))
	}

	// q1 should inherit exchange defaults
	if ex.Queues[0].dlxName != "ex-dlx" || ex.Queues[0].dlqName != "q-dlq" {
		t.Fatalf("q1 expected dlx=ex-dlx dlq=q-dlq, got dlx=%s dlq=%s", ex.Queues[0].dlxName, ex.Queues[0].dlqName)
	}
	// q2 should use overrides
	if ex.Queues[1].dlxName != "q2-dlx" || ex.Queues[1].dlqName != "q2-dlq" {
		t.Fatalf("q2 expected dlx=q2-dlx dlq=q2-dlq, got dlx=%s dlq=%s", ex.Queues[1].dlxName, ex.Queues[1].dlqName)
	}
}

func TestBuildExchangeSpecs_HandlerMapping(t *testing.T) {
	cfg := config.RabbitMQ{
		Enabled: true,
		URL:     "amqp://guest:guest@localhost:5672/",
		Exchanges: []config.RabbitExchange{
			{
				Name:    "ex",
				Kind:    "topic",
				Durable: true,
				Queues: []config.RabbitQueue{
					{Name: "password-change-queue"},
					{Name: "user-settings-updates"},
					{Name: "settings-admin-events"},
					{Name: "unknown-queue"},
				},
			},
		},
	}

	specs := buildExchangeSpecs(cfg)
	if len(specs) != 1 {
		t.Fatalf("expected 1 exchange, got %d", len(specs))
	}
	q := specs[0].Queues
	if len(q) != 4 {
		t.Fatalf("expected 4 queues, got %d", len(q))
	}

	if _, ok := q[0].Handler.(*PasswordChangeHandler); !ok {
		t.Fatalf("password-change-queue should map to PasswordChangeHandler")
	}
	if _, ok := q[1].Handler.(*UserSettingsUpdateHandler); !ok {
		t.Fatalf("user-settings-updates should map to UserSettingsUpdateHandler")
	}
	if _, ok := q[2].Handler.(*UserSettingsUpdateHandler); !ok {
		t.Fatalf("settings-admin-events should map to UserSettingsUpdateHandler")
	}
	// default mapping goes to PasswordChangeHandler per current implementation
	if _, ok := q[3].Handler.(*PasswordChangeHandler); !ok {
		t.Fatalf("unknown-queue should map to default PasswordChangeHandler")
	}
}
