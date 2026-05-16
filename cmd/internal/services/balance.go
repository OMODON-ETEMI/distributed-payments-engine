package services

import (
	"context"
	"fmt"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/internal/utilities"
)

func GetBalance(ctx context.Context, Account internal.AccountResponse, db *db.Queries) (*[]internal.BalanceResponse, error) {
	id, err := internal.StringtoPgUuid(Account.ID)
	if err != nil {
		return nil, fmt.Errorf("Error parsing ID to UUID: %w", err)
	}
	balances, err := db.GetBalancesForAccount(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("Error getting balances for account: %w", err)
	}
	out := make([]internal.BalanceResponse, 0, len(balances))
	for _, b := range balances {
		out = append(out, internal.ToBalanceResponse(Account.ID, Account.Currency, b))
	}
	resp := &out
	return resp, nil
}
