package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/routes"
	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/messaging/consumer"
	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/outbox"
	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/services"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
	"github.com/jackc/pgx/v5/pgtype"
)

func StartWebhookWorker(ctx context.Context, api *routes.ApiConfig) {
	WorkSignal := make(chan struct{}, 100)
	OutboxWorkSignal := make(chan struct{}, 100)

	// Start with exactly one worker for each task type
	go WebhookProcessor(ctx, WorkSignal, api)
	go OutboxEventProcessor(ctx, OutboxWorkSignal, api)

	go Listener(ctx, WorkSignal, OutboxWorkSignal, api)
}

func StartWithdrawalKafkWorker(ctx context.Context, api *routes.ApiConfig, consumer *consumer.KafkaConsumer) {
	topic := "withdrawal.webhook"

	if err := consumer.Subscribe(topic); err != nil {
		log.Fatalf("Failed to subscribe to Kafka topic: %v", err)
	}

	log.Printf("Kafka worker started: consuming topic %s", topic)

	for {
		msg, err := consumer.ConsumeMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Consumer poll error: %v", err)
			continue
		}

		if err := internal.ProcessWithdrawalMessage(ctx, api.Db.Queries, msg); err != nil {
			log.Printf("Failed to process message at offset %v: %v", msg.TopicPartition.Offset, err)
			continue
		}

		if err := consumer.CommitMessage(msg); err != nil {
			log.Printf("Failed to commit withdrawal message at offset %v: %v", msg.TopicPartition.Offset, err)
			continue
		}
		log.Printf("✓ Withdrawal message committed at offset %v", msg.TopicPartition.Offset)
	}
}

func StartDepositKafkaWorker(ctx context.Context, api *routes.ApiConfig, consumer *consumer.KafkaConsumer) {
	topic := "deposit.transfer"

	if err := consumer.Subscribe(topic); err != nil {
		log.Fatalf("Failed to subscribe to Kafka topic: %v", err)
	}

	log.Printf("Kafka worker started: consuming topic %s", topic)

	for {
		msg, err := consumer.ConsumeMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Consumer poll error: %v", err)
			continue
		}

		if err := internal.ProcessDepositMessage(ctx, api.Db.Queries, msg); err != nil {
			log.Printf("Failed to process message at offset %v: %v", msg.TopicPartition.Offset, err)
			continue
		}

		if err := consumer.CommitMessage(msg); err != nil {
			log.Printf("Failed to commit deposit message at offset %v: %v", msg.TopicPartition.Offset, err)
			continue
		}
		log.Printf("✓ Deposit message committed at offset %v", msg.TopicPartition.Offset)
	}
}

func WebhookProcessor(ctx context.Context, WorkSignal chan struct{}, api *routes.ApiConfig) {
	for range WorkSignal {
		webhook, err := api.Db.Queries.ListPendingIncomingWebhooks(ctx, 100)
		if err != nil || len(webhook) == 0 {
			continue
		}
		for _, w := range webhook {
			var processingErr error

			if w.EventType.String == "withdrawal.webhook" {
				var webhookData internal.WebhookBody
				if err := json.Unmarshal(w.Payload, &webhookData); err != nil {
					processingErr = fmt.Errorf("decoding withdrawal webhook: %w", err)
				} else {
					services.HandleWebhookLogic(ctx, webhookData, w, api.Db, api.Redis)
					continue
				}
			} else if w.EventType.String == "deposit.transfer" {
				_, processingErr = services.DepositLogic(ctx, w.Payload, api.Db, api.Redis)
			}

			// Handle status updates for non-autonomous logic (like Deposits)
			if processingErr != nil {
				log.Printf("Worker failed to process webhook %s: %v", w.ID, processingErr)
				api.Db.Queries.UpdateIncomingWebhookStatus(ctx, db.UpdateIncomingWebhookStatusParams{
					Status:       "failed",
					ErrorMessage: pgtype.Text{String: processingErr.Error(), Valid: true},
					ID:           w.ID,
				})
			} else {
				api.Db.Queries.UpdateIncomingWebhookStatus(ctx, db.UpdateIncomingWebhookStatusParams{
					Status: "success",
					ID:     w.ID,
				})
			}
		}
	}
}

func OutboxEventProcessor(ctx context.Context, OutboxWorkSignal chan struct{}, api *routes.ApiConfig) {
	for range OutboxWorkSignal {
		events, err := api.Db.Queries.ListPendingOutboxEvents(ctx, 100)
		if err != nil || len(events) == 0 {
			continue
		}
		for _, e := range events {
			outbox.OutboxEventKafka(ctx, e, api)
		}
	}
}

func Listener(ctx context.Context, WorkSignal chan struct{}, OutboxWorkSignal chan struct{}, api *routes.ApiConfig) {
	conn, err := api.DbPool.Acquire(ctx)
	if err != nil {
		log.Printf("Failed to acquire connection: %v", err)
	}
	defer conn.Release()

	var secondWebhookStarted bool
	var secondOutboxStarted bool

	Channels := []string{"incoming_webhooks_inserted", "outbox_events_inserted"}
	for _, ch := range Channels {
		_, err = conn.Exec(ctx, fmt.Sprintf("LISTEN %s", ch))
		if err != nil {
			log.Printf("Error listening on channel: %v", err)
		}
		fmt.Println("workers is listening for incoming webhooks ....")
	}

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Error waiting for notification: %v", err)
			continue
		}
		switch notification.Channel {
		case "incoming_webhooks_inserted":
			select {
			case WorkSignal <- struct{}{}:
				if len(WorkSignal) > 50 && !secondWebhookStarted {
					secondWebhookStarted = true
					fmt.Println("High traffic detected: Spinning up second Webhook worker")
					go WebhookProcessor(ctx, WorkSignal, api)
				}
			default:
			}
		case "outbox_events_inserted":
			select {
			case OutboxWorkSignal <- struct{}{}:
				if len(OutboxWorkSignal) > 50 && !secondOutboxStarted {
					secondOutboxStarted = true
					fmt.Println("High traffic detected: Spinning up second Outbox worker")
					go OutboxEventProcessor(ctx, OutboxWorkSignal, api)
				}
			default:
			}
		}
		fmt.Printf("Notification payload recived: %s\n", notification.Payload)
	}
}
