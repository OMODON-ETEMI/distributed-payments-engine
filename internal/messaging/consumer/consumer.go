package consumer

import (
	"context"
	"log"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type KafkaConsumer struct {
	Consumer *kafka.Consumer
}

func NewKafkaConsumer(broker, groupID string) (*KafkaConsumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":     broker,
		"group.id":              groupID,
		"auto.offset.reset":     "earliest",          // Financial system: consume ALL messages (safer)
		"enable.auto.commit":    false,               // Manual commit for reliability
		"fetch.wait.max.ms":     10,                  // Poll every 10ms for responsiveness
		"fetch.min.bytes":       1,                   // Get messages immediately, don't batch
		"session.timeout.ms":    6000,                // Faster rebalancing
		"heartbeat.interval.ms": 1000,                // More frequent heartbeats
		"isolation.level":       "read_committed",    // Only read committed messages (transactions)
	})
	if err != nil {
		return nil, err
	}
	return &KafkaConsumer{Consumer: c}, nil
}

func (wc *KafkaConsumer) Subscribe(topic string) error {
	return wc.Consumer.Subscribe(topic, nil)
}

// ConsumeMessage polls Kafka for a single message.
// It blocks until a message is received or the context is cancelled.
func (wc *KafkaConsumer) ConsumeMessage(ctx context.Context) (*kafka.Message, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			ev := wc.Consumer.Poll(5)  // ✅ Poll every 5ms instead of 10ms for faster consumption
			if ev == nil {
				continue
			}

			switch e := ev.(type) {
			case *kafka.Message:
				log.Printf("Received message: %s\n", string(e.Value))
				return e, nil
			case kafka.Error:
				if e.IsFatal() {
					return nil, e
				}
				log.Printf("Kafka Error: %v", e)
			}
		}
	}
}

// CommitMessage acknowledges that a message has been processed successfully.
func (wc *KafkaConsumer) CommitMessage(msg *kafka.Message) error {
	_, err := wc.Consumer.CommitMessage(msg)
	return err
}
