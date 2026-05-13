package outbox

import (
	"context"
	"fmt"
	"log"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
	"github.com/jackc/pgx/v5/pgtype"
)

func OutboxEventKafka(ctx context.Context, data db.OutboxEvent, api *routes.ApiConfig) error {
	payload := data.Payload

	err := api.Kafka_producer.SendMessage(data.EventType, data.PartitionKey.String, payload)
	if err != nil {
		if data.RetryCount > 5 {
			_, err := api.Db.Queries.MarkOutboxEventDeadLetter(ctx, data.ID)
			if err != nil {
				return fmt.Errorf("Error marking event as dead letter: %v", err)
			}
		}
		_, err := api.Db.Queries.MarkOutboxEventFailed(ctx, db.MarkOutboxEventFailedParams{
			ErrorCode:    pgtype.Text{String: "400", Valid: true},
			ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			ID:           data.ID,
		})
		return fmt.Errorf("error sending message to Kafka: %v", err)
	}
	_, err = api.Db.Queries.MarkOutboxEventPublished(ctx, data.ID)
	if err != nil {
		log.Printf("Warning: Message sent to Kafka but failed to update DB: %v", err)
	}

	return nil
}
