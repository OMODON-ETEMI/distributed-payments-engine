package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/internal/utilities"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func CreateAccount(ctx context.Context, params internal.AccountParameters, queries *db.Queries) (*internal.AccountResponse, error) {
	// idempotency: prefer existing by external_ref, fallback to account_number
	if params.ExternalRef != "" {
		existing, err := queries.GetAccountByExternalRef(ctx, params.ExternalRef)
		if err == nil {
			resp := internal.AccountResponseObject(existing)
			return &resp, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("error checking existing account: %w", err)
		}
	}
	if params.AccountNumber != "" {
		existing, err := queries.GetAccountByNumber(ctx, params.AccountNumber)
		if err == nil {
			resp := internal.AccountResponseObject(existing)
			return &resp, nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("error checking existing account by number: %w", err)
		}
	}

	customerID, err := internal.StringtoPgUuid(params.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("error parsing customer ID: %w", err)
	}

	if params.Metadata == nil {
		params.Metadata = make(map[string]interface{})
	}
	metadataBytes, err := json.Marshal(params.Metadata)
	if err != nil {
		return nil, fmt.Errorf("error parsing metadata: %w", err)
	}

	openedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	account, err := queries.CreateAccount(ctx, db.CreateAccountParams{
		CustomerID:       customerID,
		ExternalRef:      params.ExternalRef,
		AccountNumber:    params.AccountNumber,
		AccountType:      params.AccountType,
		Status:           params.Status,
		CurrencyCode:     params.CurrencyCode,
		LedgerNormalSide: params.LedgerNormalSide,
		Metadata:         metadataBytes,
		OpenedAt:         openedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating Account: %w", err)
	}

	resp := internal.AccountResponseObject(account)
	return &resp, nil
}

func GetAccountByAccountNumber(ctx context.Context, AccountNumber string, queries *db.Queries) (*internal.AccountResponse, error) {
	account, err := queries.GetAccountByNumber(ctx, AccountNumber)
	if err != nil {
		return nil, err
	}
	resp := internal.AccountResponseObject(account)
	return &resp, nil
}

func GetAccountByID(ctx context.Context, ID string, queries *db.Queries) (*internal.AccountResponse, error) {
	id, err := internal.StringtoPgUuid(ID)
	if err != nil {
		return nil, fmt.Errorf("Error parsing ID to UUID: %w", err)
	}
	account, err := queries.GetAccountByID(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := internal.AccountResponseObject(account)
	return &resp, nil
}

func ListAccountsByCustomer(ctx context.Context, params internal.AccountParameters, queries *db.Queries) ([]internal.AccountResponse, error) {
	if params.CustomerID == "" {
		return nil, fmt.Errorf("customer_id is required")
	}

	id, err := internal.StringtoPgUuid(params.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("invalid customer ID: %w", err)
	}

	limit := int32(50)
	offset := int32(0)
	if params.Limit > 0 {
		limit = int32(params.Limit)
	}
	if params.Offset > 0 {
		offset = int32(params.Offset)
	}
	if limit > 1000 {
		limit = 1000
	}

	accounts, err := queries.ListAccountsByCustomer(ctx, db.ListAccountsByCustomerParams{
		Limit:      limit,
		Offset:     offset,
		CustomerID: id,
	})
	if err != nil {
		return nil, err
	}

	out := make([]internal.AccountResponse, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, internal.AccountResponseObject(a))
	}
	return out, nil
}

func UpdateAccountStatus(ctx context.Context, params internal.AccountParameters, queries *db.Queries) (*internal.AccountResponse, error) {
	if params.Status == "" {
		return nil, fmt.Errorf("status is required")
	}

	id, err := internal.StringtoPgUuid(params.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid account ID: %w", err)
	}

	var closedAt pgtype.Timestamptz
	if params.Status == "closed" {
		closedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	} else {
		closedAt = pgtype.Timestamptz{Valid: false}
	}

	account, err := queries.UpdateAccountStatus(ctx, db.UpdateAccountStatusParams{
		Status:   params.Status,
		ClosedAt: closedAt,
		ID:       id,
	})
	if err != nil {
		return nil, err
	}

	resp := internal.AccountResponseObject(account)
	return &resp, nil
}
