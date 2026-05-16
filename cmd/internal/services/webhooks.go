package services

import (
	"context"
	"log"

	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database"
	db "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	repositry "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/internal/repositry"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/internal/utilities"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
)

func HandleWebhookLogic(ctx context.Context, data internal.WebhookBody, webhook db.IncomingWebhook, d *database.Db, rdb *redis.Client) {
	var err error
	switch data.Event {
	case "transfer.success":
		err = repositry.HandleTransferSuccess(ctx, data.Data, d, rdb)
	case "transfer.failed", "transfer.reversed":
		repositry.HandleTransferFailed(ctx, data.Data, d, rdb)
	default:
	}
	if err != nil {
		log.Printf("Worker failed to process webhook %s: %v", data.ID, err)
		_, err := d.Queries.UpdateIncomingWebhookStatus(ctx, db.UpdateIncomingWebhookStatusParams{
			Status:       "failed",
			ErrorMessage: pgtype.Text{String: err.Error(), Valid: true},
			ID:           webhook.ID,
		})
		if err != nil {
			log.Printf("Worker failed to update webhook status: %v", err)
		}
		return
	}
	_, err = d.Queries.UpdateIncomingWebhookStatus(ctx, db.UpdateIncomingWebhookStatusParams{
		Status: "success",
		ID:     webhook.ID,
	})
}
