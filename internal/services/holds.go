package services

import (
	"context"
	"fmt"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
)

func ConsumeHold(ctx context.Context, params internal.HoldParams, queries *db.Queries) error {
	amount, err := internal.StringToNumeric(params.Amount)
	if err != nil {
		return fmt.Errorf("invalid amount: %w", err)
	}

	id, err := internal.StringtoPgUuid(params.ID)
	if err != nil {
		return fmt.Errorf("invalid hold ID: %w", err)
	}

	// Execute consumption logic
	_, err = queries.ConsumeHold(ctx, db.ConsumeHoldParams{
		Amount: amount,
		ID:     id,
	})
	if err != nil {
		return fmt.Errorf("failed to consume hold: %w", err)
	}

	_, err = queries.ReleaseHold(ctx, db.ReleaseHoldParams{
		Amount: amount,
		ID:     id,
	})
	if err != nil {
		return fmt.Errorf("failed to release remaining hold: %w", err)
	}

	return nil
}

func GetHoldByTransferRequest(ctx context.Context, transferID string, queries *db.Queries) (*internal.HoldResponse, error) {
	trfId, err := internal.StringtoPgUuid(transferID)
	if err != nil {
		return nil, fmt.Errorf("invalid transfer ID: %w", err)
	}

	hold, err := queries.GetActiveHoldByTransferRequestID(ctx, trfId)
	if err != nil {
		return nil, err
	}

	resp := internal.ToHoldResponse(hold)
	return &resp, nil
}
