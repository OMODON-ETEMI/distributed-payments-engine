package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database"
	db "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/repositry"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

// Router defines the interface for the payment provider routing logic
type Router interface {
	Route(ctx context.Context, req internal.InitiateRequest) (*internal.InitiateResponse, error)
}

// WithdrawalLogic handles the core business logic for processing a withdrawal.
func WithdrawalLogic(ctx context.Context, payload []byte, d *database.Db, rdb *redis.Client, router Router) (*internal.WithdrawalResponse, error) {
	requestHash := internal.HashRequest(payload)
	params := internal.WithdrawParams{}
	if err := json.Unmarshal(payload, &params); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %v", err)
	}

	customerID, err := internal.StringtoPgUuid(params.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("error parsing customer ID: %v", err)
	}
	customer, err := d.Queries.GetCustomerByID(ctx, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("customer not found")
		}
		return nil, fmt.Errorf("error looking up customer: %v", err)
	}

	sourceAccountID, err := internal.StringtoPgUuid(params.SourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("error parsing source account ID: %v", err)
	}
	sourceAccount, err := d.Queries.GetAccountByID(ctx, sourceAccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("source account not found")
		}
		return nil, fmt.Errorf("error looking up source account: %v", err)
	}

	if sourceAccount.Status != "active" {
		return nil, fmt.Errorf("source account is not active")
	}
	if params.CurrencyCode != sourceAccount.CurrencyCode {
		return nil, fmt.Errorf("currency code mismatch")
	}

	amount, err := internal.StringToNumeric(params.Amount)
	feeAmount, err := internal.StringToNumeric(params.FeeAmount)
	metaBytes := []byte("null")
	if params.Metadata != nil {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			return nil, fmt.Errorf("error parsing metadata: %v", err)
		}
		metaBytes = b
	}

	check, err := internal.IdemCheck(ctx, d.Queries, rdb, params.IdempotencyKeyID, params.CustomerID, requestHash, "withdraw_create")
	if err != nil {
		return nil, fmt.Errorf("error checking idempotency key: %v", err)
	}
	if !check.ShouldProceed {
		var cachedData internal.WithdrawalResponse
		if err := json.Unmarshal(check.CachedResponse, &cachedData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cached response")
		}
		return &cachedData, nil
	}

	idempkey, err := d.Queries.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
		IdempotencyKey: params.IdempotencyKeyID,
		Scope:          "withdraw_create",
		RequestHash:    []byte(requestHash),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(24 * time.Hour),
			Valid: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error creating idempotency key: %v", err)
	}

	var trf db.TransferRequest
	err = d.ExecTx(ctx, func(q *db.Queries) error {
		sourceAcct, err := q.GetAccountByIDForUpdate(ctx, sourceAccount.ID)
		if err != nil {
			return fmt.Errorf("error looking up source account: %v", err)
		}
		if sourceAcct.Status != "active" {
			return fmt.Errorf("account is not active")
		}

		settlement, err := q.GetAccountByExternalRef(ctx, "system_settlement_ngn")
		if err != nil {
			return fmt.Errorf("error looking up system settlement account: %v", err)
		}

		balance, err := internal.GetOrCreateBalanceProjection(ctx, q, sourceAcct.ID, params.CurrencyCode, "available")
		if err != nil {
			return fmt.Errorf("getting balance: %w", err)
		}

		avail, _ := decimal.NewFromString(internal.NumericToString(balance.AvailableBalance))
		amt, _ := decimal.NewFromString(internal.NumericToString(amount))
		if avail.LessThan(amt) {
			return fmt.Errorf("insufficient funds")
		}

		trf, err = q.CreateTransferRequest(ctx, db.CreateTransferRequestParams{
			IdempotencyKeyID:     idempkey.ID,
			CustomerID:           customer.ID,
			SourceAccountID:      sourceAccount.ID,
			DestinationAccountID: settlement.ID,
			CurrencyCode:         params.CurrencyCode,
			Amount:               amount,
			FeeAmount:            feeAmount,
			Metadata:             metaBytes,
		})
		if err != nil {
			return fmt.Errorf("error creating transfer request for withdrawal: %w", err)
		}

		jtx, err := q.CreateJournalTransaction(ctx, db.CreateJournalTransactionParams{
			TransactionRef:    uuid.NewString(),
			TransferRequestID: trf.ID,
			IdempotencyKeyID:  idempkey.ID,
			Status:            "pending",
			EntryType:         "hold",
			AccountingDate:    pgtype.Date{Time: trf.RequestedAt.Time, Valid: true},
			EffectiveAt:       pgtype.Timestamptz{Time: trf.RequestedAt.Time, Valid: true},
			SourceSystem:      "core",
			SourceEventID:     pgtype.Text{String: trf.ID.String(), Valid: true},
			Description:       pgtype.Text{String: params.Description, Valid: params.Description != ""},
			Metadata:          metaBytes,
		})
		if err != nil {
			return fmt.Errorf("error creating journal transaction: %v", err)
		}

		legs := []internal.JournalLeg{
			{AccountID: sourceAccount.ID, BalanceKind: "available", Amount: amount, Side: "debit"},
			{AccountID: sourceAccount.ID, BalanceKind: "held", Amount: amount, Side: "credit"},
		}
		if err := internal.ValidateLedgerBalance(legs); err != nil {
			return err
		}

		var sourceLineID pgtype.UUID
		for index, leg := range legs {
			line, err := q.CreateJournalLine(ctx, db.CreateJournalLineParams{
				JournalTransactionID: jtx.ID,
				LineNumber:           int32(index) + 1,
				AccountID:            leg.AccountID,
				Side:                 leg.Side,
				Amount:               leg.Amount,
				CurrencyCode:         params.CurrencyCode,
				BalanceKind:          leg.BalanceKind,
				Memo:                 pgtype.Text{String: "funds reserved for withdrawal", Valid: true},
				Metadata:             metaBytes,
			})
			if err != nil {
				return fmt.Errorf("error creating journal line: %v", err)
			}
			if leg.AccountID == sourceAccount.ID && leg.Side == "debit" {
				sourceLineID = line.ID
			}
		}

		_, err = q.CreateHold(ctx, db.CreateHoldParams{
			AccountID:            sourceAccount.ID,
			TransferRequestID:    trf.ID,
			JournalTransactionID: jtx.ID,
			IdempotencyKeyID:     idempkey.ID,
			Status:               "active",
			CurrencyCode:         params.CurrencyCode,
			Amount:               amount,
			RemainingAmount:      amount,
			ReleasedAmount:       pgtype.Numeric{Int: big.NewInt(0), Valid: true},
			CapturedAmount:       pgtype.Numeric{Int: big.NewInt(0), Valid: true},
			ReasonCode:           pgtype.Text{String: "external_withdrawal", Valid: true},
			Reason:               pgtype.Text{String: params.Description, Valid: true},
			ExpiresAt:            pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
		})

		destLedger, _ := decimal.NewFromString(internal.NumericToString(balance.LedgerBalance))
		destHeld, _ := decimal.NewFromString(internal.NumericToString(balance.HeldBalance))
		newDestHeld := destHeld.Add(amt)
		newDestAvail := destLedger.Sub(newDestHeld)

		newDestLedgerNum, _ := internal.StringToNumeric(destLedger.String())
		newDestAvailNum, _ := internal.StringToNumeric(newDestAvail.String())
		newDestHeldNum, _ := internal.StringToNumeric(newDestHeld.String())

		err = q.UpsertBalanceProjectionWithExpectedVersion(ctx, db.UpsertBalanceProjectionWithExpectedVersionParams{
			AccountID:        sourceAccount.ID,
			CurrencyCode:     params.CurrencyCode,
			BalanceKind:      "available",
			LedgerBalance:    newDestLedgerNum,
			AvailableBalance: newDestAvailNum,
			HeldBalance:      newDestHeldNum,
			LastTxID:         jtx.ID,
			LastLineID:       sourceLineID,
			ExpectedVersion:  balance.Version,
		})
		if err != nil {
			return fmt.Errorf("upserting source balance: %w", err)
		}

		payloadBytes, err := json.Marshal(internal.WithdrawalResponse{
			Transfer: internal.ToTransferResponse(trf, nil),
			Message:  "withdrawal initiated, processing",
		})
		return internal.SaveIdem(ctx, q, rdb, params.CustomerID, params.IdempotencyKeyID, idempkey.ID, payloadBytes, 202)
	})

	if err != nil {
		return nil, err
	}

	providerResp, err := router.Route(ctx, internal.InitiateRequest{
		Amount:        params.Amount,
		Currency:      params.CurrencyCode,
		RecipientCode: params.DestinationAccountID,
		Reference:     trf.ID.String(),
		Reason:        params.Description,
	})

	if err != nil {
		jsonData, _ := json.Marshal(internal.WebhookTransferData{
			ID:            trf.CustomerID.String(),
			Reference:     trf.ID.String(),
			Status:        "failed",
			FailureReason: err.Error(),
		})
		repositry.HandleTransferFailed(ctx, json.RawMessage(jsonData), d, rdb)
		return nil, err
	}

	return &internal.WithdrawalResponse{
		Transfer:         internal.ToTransferResponse(trf, nil),
		ProviderResponse: providerResp,
		Message:          "withdrawal initiated, processing",
	}, nil
}
