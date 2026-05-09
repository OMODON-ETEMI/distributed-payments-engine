package producer

import (
	"fmt"
	"log"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

type KafkaProducer struct {
	Producer *kafka.Producer
}

func NewKafkaProducer(broker string) (*KafkaProducer, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":  broker,
		"acks":               "all",
		"retries":            5,
		"linger.ms":          100,
		"enable.idempotence": true,
		"partitioner":        "consistent_random",
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating Kafka producer: %v", err)
	}
	kp := &KafkaProducer{
		Producer: p,
	}
	go kp.listenForEvents()
	return kp, nil
}

func (kp *KafkaProducer) listenForEvents() {
	for e := range kp.Producer.Events() {
		switch ev := e.(type) {
		case *kafka.Message:
			if ev.TopicPartition.Error != nil {
				log.Printf("Delivery failed: %v\n", ev.TopicPartition.Error)
			} else {
				topicName := "unknown"
				if ev.TopicPartition.Topic != nil {
					topicName = *ev.TopicPartition.Topic
				}
				log.Printf("Message delivered to topic %s [%d] at offset %v\n",
					topicName, ev.TopicPartition.Partition, ev.TopicPartition.Offset)
			}
		}
	}
}

func (kp *KafkaProducer) SendMessage(topic string, key string, payload []byte) error {
	err := kp.Producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: int32(kafka.PartitionAny),
		},
		Key:   []byte(key),
		Value: payload,
	}, nil)
	if err != nil {
		return fmt.Errorf("Error sending message: %v", err)
	}
	return nil
}

func (kp *KafkaProducer) Close() {
	kp.Producer.Flush(15 * 1000)
	kp.Producer.Close()
}
