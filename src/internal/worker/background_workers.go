package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/OMODON-ETEMI/distributed-payments-engine/src/internal/outbox"
	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
)

func StartWebhookWorker(ctx context.Context, api *routes.ApiConfig) {
	WorkSignal := make(chan struct{}, 100)
	OutboxWorkSignal := make(chan struct{}, 100)

	// Start with exactly one worker for each task type
	go WebhookProcessor(ctx, WorkSignal, api)
	go OutboxEventProcessor(ctx, OutboxWorkSignal, api)

	go Listener(ctx, WorkSignal, OutboxWorkSignal, api)
}

func WebhookProcessor(ctx context.Context, WorkSignal chan struct{}, api *routes.ApiConfig) {
	var webhookData routes.WebhookBody
	for range WorkSignal {
		webhook, err := api.Db.Queries.ListPendingIncomingWebhooks(ctx, 100)
		if err != nil || len(webhook) == 0 {
			continue
		}
		for _, w := range webhook {
			err := json.Unmarshal(w.Payload, &webhookData)
			if err != nil {
				fmt.Printf("Error decoding webhook: %v", err)
				continue
			}
			api.HandleWebhookLogic(ctx, webhookData, w)
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
