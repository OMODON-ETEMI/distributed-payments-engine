package worker

import (
	"context"
	"time"

	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/routes"
)

func StartOutboxEventReaper(ctx context.Context, api *routes.ApiConfig) {
	ticker := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-ticker.C:
			api.Db.Queries.ResetStuckOutboxEvent(ctx)
		case <-ctx.Done():
			return
		}
	}
}
