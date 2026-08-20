# Milvus CDC Library [![GoDoc][doc-img]][doc] [![GoReport][go-report-img]][go-report]

``milvus-cdc`` is a Go library for Change Data Capture (CDC) with Milvus: it replicates
writes (insert, delete, create/drop collection, create/drop partition, create/drop
index) to one or more Milvus clusters as they happen. Now, the library supports Milvus
1.x versions.

Every broker shares the same retry/panic-recovery core and sits behind the same
`IBrokerFactory` interface, so swapping the transport underneath — Redis, a plain Go
channel, RabbitMQ, MQTT, or Kafka — doesn't change how you wire up sync logic.

<div align="center">
  <img src="https://media.giphy.com/media/ydPWRTNMR3x8mAWTdk/giphy.gif">
</div>

Installation
------------
    go get github.com/warriors-vn/milvus-cdc

Usage
-----

You can import ``milvus-cdc`` using a basic statement:

```go
import "github.com/warriors-vn/milvus-cdc"
```

Every broker replicates each CDC message to all the `IMilvusClientInterface` clients
you give it, and is wired into a `WorkerCDC` the same way regardless of transport —
only the broker construction changes between examples below:

```go
milvusCli, err := cdc.NewMilvusClient("0.0.0.0", "19530", cdc.DefaultTimeout)
if err != nil {
	log.Fatal(err)
}

// redisBroker, channelBroker, rabbitBroker, mqttBroker or kafkaBroker — see below
broker := cdc.NewBrokerFactory(map[string]cdc.IBrokerFactory{cdc.Redis: redisBroker})
worker := cdc.NewWorkerCDC(broker)

go worker.Start(cdc.Redis, "cdc-channel", cdc.Queue)
defer worker.Stop(cdc.Redis)
```

There are several broker transports supported by milvus-cdc: Redis (`PubSub` / `Queue`
/ `Stream` patterns), a plain Go channel, RabbitMQ, MQTT, and Kafka.

*Some examples:*

<h3>cdc.RedisBroker (Queue)</h3>
Broadcasts messages popped off a Redis list via BRPop to every milvus client; a message is gone the instant it's popped, whether or not it's ever successfully applied.<br>

```go
redisBroker := cdc.NewRedisBroker(cdc.NewRedisClient(redisCli), []cdc.IMilvusClientInterface{milvusCli})

go worker.Start(cdc.Redis, "cdc-channel", cdc.Queue)

cdc.NewRedisClient(redisCli).LPush(context.Background(), "cdc-channel", string(msg))
```

<h3>cdc.RedisBroker (Stream)</h3>
Same replication as Queue, but backed by Redis Streams consumer groups: a message is only XACKed once every milvus client applied it, otherwise it stays a reclaimable pending entry.<br>

```go
redisBroker := cdc.NewRedisBroker(cdc.NewRedisClient(redisCli), []cdc.IMilvusClientInterface{milvusCli},
	cdc.WithStreamConsumer("worker-1")) // unique per broker instance sharing the group

go worker.Start(cdc.Redis, "cdc-stream", cdc.Stream)

cdc.NewRedisClient(redisCli).XAdd(context.Background(), "cdc-stream", map[string]interface{}{
	cdc.StreamPayloadField: string(msg),
})
```

<h3>cdc.ChannelBroker</h3>
Reads off a plain Go channel instead of an external broker — useful for tests or a same-process producer.<br>

```go
msgCh := make(chan string)
channelBroker := cdc.NewChannelBroker(msgCh, []cdc.IMilvusClientInterface{milvusCli})

go channelBroker.Start("", "") // channel/pattern args are unused, kept to satisfy IBrokerFactory

msgCh <- string(msg)
```

<h3>cdc.RabbitMQBroker</h3>
Acks a delivery only once every milvus client applied it; nacks and requeues it on failure, giving real at-least-once delivery across a restart.<br>

```go
rabbitBroker := cdc.NewRabbitMQBroker(cdc.NewRabbitMQClient(ch), []cdc.IMilvusClientInterface{milvusCli})

go worker.Start(cdc.RabbitMQ, "cdc-queue", cdc.Queue) // only cdc.Queue is supported
```

<h3>cdc.MQTTBroker</h3>
For EMQX, Mosquitto, HiveMQ, etc. Acks manually once every milvus client succeeds; needs QoS >= 1 and the client built with SetAutoAckDisabled(true) — MQTT has no nack, so a failed message is simply left unacked.<br>

```go
mqttBroker := cdc.NewMQTTBroker(cdc.NewMQTTClient(mqttCli, 1), []cdc.IMilvusClientInterface{milvusCli}) // QoS 1

go worker.Start(cdc.MQTT, "cdc-topic", cdc.Queue) // only cdc.Queue is supported
```

<h3>cdc.KafkaBroker</h3>
Commits the consumer-group offset only once every milvus client applied it; the reader must be built with a GroupID for this to mean anything.<br>

```go
kafkaBroker := cdc.NewKafkaBroker(cdc.NewKafkaClient(reader, nil), []cdc.IMilvusClientInterface{milvusCli})

go worker.Start(cdc.Kafka, "cdc-topic", cdc.Queue) // topic is unused: it's fixed on the reader; only cdc.Queue is supported
```

Useful options shared by every broker's constructor (all optional):

- `With{Redis,Channel,RabbitMQ,MQTT,Kafka}OnMessage(func(msg string, idx int, err error))` — called after every message is applied to a milvus client; `err` is `nil` on success. Nothing is logged unless you set this.
- `WithOnConnError` / `WithRabbitMQOnConnError` / `WithKafkaOnConnError(func(err error))` — called when the underlying read itself fails (connection dropped). Not called on a graceful `Stop()`. MQTT and the Go channel have no connection to lose, so they don't take one.
- `With{...}MaxRetries(n int)` / `With{...}RetryDelay(d time.Duration)` — override the default retry policy (3 attempts, 500ms apart) applied when a milvus client call fails.

Configuration
-------------

Create a configuration file to specify the Milvus server connection details, event types,
and other CDC options.

Example:

```yaml
version: '3.8'

services:
  milvus-one:
    image: milvusdb/milvus:1.1.1-cpu-d061621-330cc6
    container_name: milvus-one
    ports:
      - "19530:19530"  # Milvus API port
      - "19121:19121"  # Monitoring port
    environment:
      DEPLOY_MODE: standalone  # Configure Milvus to run in standalone mode
    volumes:
      - milvus-data-one:/var/lib/milvus
      - ./server_config.yaml:/var/lib/milvus/conf/server_config.yaml

  milvus-two:
    image: milvusdb/milvus:1.1.1-cpu-d061621-330cc6
    container_name: milvus-two
    ports:
      - "29530:19530"  # Milvus API port
      - "29121:19121"  # Monitoring port
    environment:
      DEPLOY_MODE: standalone  # Configure Milvus to run in standalone mode
    volumes:
      - milvus-data-two:/var/lib/milvus
      - ./server_config.yaml:/var/lib/milvus/conf/server_config.yaml

  milvus-three:
    image: milvusdb/milvus:1.1.1-cpu-d061621-330cc6
    container_name: milvus-three
    ports:
      - "39530:19530"  # Milvus API port
      - "39121:19121"  # Monitoring port
    environment:
      DEPLOY_MODE: standalone  # Configure Milvus to run in standalone mode
    volumes:
      - milvus-data-three:/var/lib/milvus
      - ./server_config.yaml:/var/lib/milvus/conf/server_config.yaml

  milvus-em:
    image: milvusdb/milvus-em:latest
    container_name: milvus-em
    ports:
      - "3001:80"  # Milvus API port

volumes:
  milvus-data-one:
  milvus-data-two:
  milvus-data-three:
```

Troubleshooting
---------------

- **Invalid CPU cache size error:** Adjust cache.cache_size in server_config.yaml to fit within system memory.
- **Connection issues:** Ensure Milvus server is running and accessible at the specified host and port.
- **Event lag:** Increase polling_interval in ``milvus-cdc-config.yaml`` to reduce system load.

Performance
-----------

Don't hesitate to participate in the discussion to enhance the generic helpers implementations.

Contributing
------------

* Ping me on instagram [@tuanelnino9](https://www.instagram.com/tuanelnino9) or [Facebook](https://www.facebook.com/tuanelnino9) or Linkedin [Tuan Nguyen Van](https://www.linkedin.com/in/tuan-nguyen-van-555315156) DMs, mentions, whatever, [send email](mailto:nguyenvantuan2391996@gmail.com) :))
* Fork the [project](https://github.com/warriors-vn/milvus-cdc)
* Fix [open issues](https://github.com/warriors-vn/milvus-cdc/issues) or request new features

Don't hesitate :))

Authors
-------

* Tuan Nguyen Van

[doc]: https://pkg.go.dev/github.com/warriors-vn/milvus-cdc
[doc-img]: https://pkg.go.dev/badge/github.com/warriors-vn/milvus-cdc
[go-report]: https://goreportcard.com/report/github.com/warriors-vn/milvus-cdc
[go-report-img]: https://goreportcard.com/badge/github.com/warriors-vn/milvus-cdc
