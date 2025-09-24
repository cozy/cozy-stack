## RabbitMQ

Integration with RabbitMQ: configuration, topology, and handler semantics.

### Overview

Cozy Stack can consume messages from RabbitMQ and dispatch them to Go handlers. The consumer is managed by a background manager which:

- establishes and monitors the AMQP connection (with optional TLS),
- declares exchanges and queues (if configured to do so),
- binds queues to routing keys,
- starts per-queue consumers with QoS and redelivery limits,
- dispatches deliveries to queue-specific handlers.

### Configuration

RabbitMQ is configured in `cozy.yaml` under the `rabbitmq` key.

Key fields:

- `enabled`: Enable the consumer.
- `url`: AMQP URL, e.g. `amqp://guest:guest@localhost:5672/`.
- `tls`: Optional TLS settings (`root_ca`, `insecure_skip_validation`, `server_name`).
- `exchanges[]`: List of exchanges the stack should consume from.
  - `name`: Exchange name.
  - `kind`: Exchange type (e.g. `topic`).
  - `durable`: Whether the exchange is durable.
  - `declare_exchange`: If true, the exchange is declared on startup.
  - `dlx_name`, `dlq_name`: Optional defaults for queues under this exchange.
  - `queues[]`: List of queues to consume.
    - `name`: Queue name.
    - `bindings[]`: Routing keys to bind to the exchange.
    - `declare`: If true, declare the queue on startup.
    - `prefetch`: Per-consumer QoS prefetch.
    - `delivery_limit`: x-delivery-limit for quorum queues.
    - `declare_dlx`: If true, declare the Dead Letter Exchange (DLX) on startup.
    - `declare_dlq`: If true, declare the Dead Letter Queue (DLQ) on startup.
    - `dlx_name`, `dlq_name`: Optional overrides per queue.

Example:

```yaml
rabbitmq:
  enabled: true
  url: amqp://guest:guest@localhost:5672/
  tls:
    # root_ca: /etc/ssl/certs/ca.pem
    insecure_skip_validation: false
    # server_name: rabbit.internal
  exchanges:
    - name: auth
      kind: topic
      durable: true
      declare_exchange: true
      dlx_name: auth.dlx
      dlq_name: auth.dlq
      queues:
        - name: user.password.updated
          declare: true
          declare_dlx: true
          declare_dlq: true
          prefetch: 8
          delivery_limit: 5
          bindings:
            - password.changed
        - name: user.created
          declare: true
          declare_dlx: false
          declare_dlq: true
          prefetch: 8
          delivery_limit: 5
          bindings:
            - user.created
```

### Dead Letter Exchange (DLX) and Dead Letter Queue (DLQ)

The RabbitMQ integration supports Dead Letter Exchange (DLX) and Dead Letter Queue (DLQ) functionality for handling failed messages:

- **DLX (Dead Letter Exchange)**: An exchange where messages are sent when they cannot be delivered to their original destination.
- **DLQ (Dead Letter Queue)**: A queue bound to the DLX that receives failed messages for analysis or reprocessing.

#### Configuration

DLX and DLQ can be configured at both exchange and queue levels:

- **Exchange level**: Set `dlx_name` and `dlq_name` in the exchange configuration to provide defaults for all queues under that exchange.
- **Queue level**: Set `dlx_name` and `dlq_name` in the queue configuration to override exchange defaults.
- **Declaration control**: Use `declare_dlx` and `declare_dlq` flags to control whether the DLX and DLQ are automatically declared on startup.

#### Example with DLX/DLQ

```yaml
rabbitmq:
  enabled: true
  url: amqp://guest:guest@localhost:5672/
  exchanges:
    - name: auth
      kind: topic
      durable: true
      declare_exchange: true
      dlx_name: auth.dlx
      dlq_name: auth.dlq
      queues:
        - name: user.password.updated
          declare: true
          declare_dlx: true
          declare_dlq: true
          prefetch: 8
          delivery_limit: 5
          bindings:
            - password.changed
        - name: user.created
          declare: true
          declare_dlx: false
          declare_dlq: true
          dlq_name: user.created.dlq  # Override exchange default
          prefetch: 8
          delivery_limit: 5
          bindings:
            - user.created
```

#### Behavior

- When `declare_dlx: true` and a `dlx_name` is provided, the DLX is declared as a fanout exchange on startup.
- When `declare_dlq: true` and a `dlq_name` is provided, the DLQ is declared and bound to the DLX on startup.
- If queue-level `dlx_name`/`dlq_name` are not specified, exchange-level defaults are used.
- Messages that exceed the `delivery_limit` or are rejected will be sent to the DLX and routed to the DLQ.

### Handlers

Handlers implement a simple interface:

```go
type Handler interface {
    Handle(ctx context.Context, d amqp.Delivery) error
}
```

Returning `nil` acknowledges the message. Returning a non-nil error causes the message to be requeued (subject to broker policies and delivery limits).

Queue names are mapped to handlers in the stack. For example:

- `user.password.updated` → updates an instance passphrase when a `password.changed` routing key is received.
- `user.created` → validates and processes user creation events.

Message schemas are JSON and validated in the handler. Example payload for `user.password.updated`:

```json
{
  "twakeId": "string",
  "iterations": 100000,
  "hash": "base64",
  "publicKey": "base64",
  "privateKey": "cipherString",
  "key": "cipherString",
  "timestamp": 1726040000,
  "domain": "example.cozy.cloud"
}
```

Example payload for `user.created`:

```json
{
  "twakeId": "string",
  "mobile": "string",
  "internalEmail": "string",
  "iterations": 100000,
  "hash": "base64",
  "publicKey": "base64",
  "privateKey": "cipherString",
  "key": "cipherString",
  "timestamp": 1726040000
}
```

### Lifecycle

On startup, if `rabbitmq.enabled` is true:

1. The manager creates an AMQP connection (TLS if configured) and retries with exponential backoff.
2. It declares configured exchanges and queues (if `declare_*` flags are set).
3. It declares Dead Letter Exchanges and Dead Letter Queues (if `declare_dlx`/`declare_dlq` flags are set).
4. It binds queues to their routing keys and starts consumers.
5. It exposes a readiness channel internally so tests can wait until consumption is active.
6. It monitors the connection and restarts consumers upon reconnection.

### Adding a new queue handler

Follow these steps to introduce a new queue and its handler.

1) Define the message schema and the handler

Create a handler type that implements the `Handle(ctx, d)` method and an accompanying message struct.

```go
// Example message payload consumed from the queue
type ExampleEvent struct {
    ID        string `json:"id"`
    Action    string `json:"action"`
    Timestamp int64  `json:"timestamp"`
}

// ExampleHandler processes ExampleEvent messages
type ExampleHandler struct{}

func NewExampleHandler() *ExampleHandler { return &ExampleHandler{} }

func (h *ExampleHandler) Handle(ctx context.Context, d amqp.Delivery) error {
    var msg ExampleEvent
    if err := json.Unmarshal(d.Body, &msg); err != nil {
        return fmt.Errorf("example: invalid payload: %w", err)
    }
    if msg.ID == "" {
        return fmt.Errorf("example: missing id")
    }
    // TODO: implement business logic
    return nil // ack
}
```

2) Register the handler for a queue name

Map the queue name to the handler so the manager knows which handler to use. This usually happens where queues are built from config.

```go
switch configQueue.Name {
case "example.queue":
    handler = NewExampleHandler()
}
```

3) Configure the exchange and queue in `cozy.yaml`

Add the queue under an exchange with the routing keys to bind.

```yaml
rabbitmq:
  enabled: true
  url: amqp://guest:guest@localhost:5672/
  exchanges:
    - name: auth
      kind: topic
      durable: true
      declare_exchange: true
      queues:
        - name: example.queue
          declare: true
          prefetch: 8
          delivery_limit: 5
          bindings:
            - example.created
            - example.updated
```

4) Publish a message (example)

```go
ch.PublishWithContext(ctx,
    "auth",            // exchange
    "example.created", // routing key
    false, false,
    amqp.Publishing{
        DeliveryMode: amqp.Persistent,
        ContentType:  "application/json",
        Body:         []byte(`{"id":"123","action":"created","timestamp":1726040000}`),
    },
)
```


