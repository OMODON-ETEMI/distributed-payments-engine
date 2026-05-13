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
		"bootstrap.servers":  broker,
		"group.id":           groupID,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
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
			// We use a short timeout (100ms) so the loop can
			// frequently check if the context has been cancelled.
			ev := wc.Consumer.Poll(100)
			if ev == nil {
				continue
			}

			switch e := ev.(type) {
			case *kafka.Message:
				// Return the specific message instance 'e'
				return e, nil
			case kafka.Error:
				// If the error is fatal, we should stop and return it
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
