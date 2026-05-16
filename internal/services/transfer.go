package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database"
	db "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

func CreateTransfer(ctx context.Context, params internal.TransferParams, d *database.Db, rdb *redis.Client, requestHash string) (*internal.TransferResponse, error) {
	// 1. Idempotency Check
	check, err := internal.IdemCheck(ctx, d.Queries, rdb, params.IdempotencyKeyID, params.CustomerID, requestHash, "transfer_create")
	if err != nil {
		return nil, fmt.Errorf("error checking idempotency key: %w", err)
	}
	if !check.ShouldProceed {
		var cachedData internal.TransferResponse
		if err := json.Unmarshal(check.CachedResponse, &cachedData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cached response: %w", err)
		}
		return &cachedData, nil
	}

	// 2. Validate basic entities exist before starting Tx
	customerID, err := internal.StringtoPgUuid(params.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("invalid customer ID: %w", err)
	}
	customer, err := d.Queries.GetCustomerByID(ctx, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("customer not found")
		}
		return nil, fmt.Errorf("error looking up customer: %w", err)
	}

	sourceAccountID, err := internal.StringtoPgUuid(params.SourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("invalid source account ID: %w", err)
	}
	destAccountID, err := internal.StringtoPgUuid(params.DestinationAccountID)
	if err != nil {
		return nil, fmt.Errorf("invalid destination account ID: %w", err)
	}

	amount, err := internal.StringToNumeric(params.Amount)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}
	feeAmount, err := internal.StringToNumeric(params.FeeAmount)
	if err != nil {
		return nil, fmt.Errorf("invalid fee amount: %w", err)
	}

	metaBytes, err := json.Marshal(params.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// 3. Create Idempotency Key record
	idempkey, err := d.Queries.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
		IdempotencyKey: params.IdempotencyKeyID,
		Scope:          "transfer_create",
		RequestHash:    []byte(requestHash),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(24 * time.Hour),
			Valid: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error creating idempotency key record: %w", err)
	}

	var trf db.TransferRequest
	var jtx db.JournalTransaction

	// 4. Execute Transaction
	err = d.ExecTx(ctx, func(q *db.Queries) error {
		// Lock accounts
		sourceAcct, err := q.GetAccountByIDForUpdate(ctx, sourceAccountID)
		if err != nil {
			return fmt.Errorf("error locking source account: %w", err)
		}
		if sourceAcct.Status != "active" {
			return fmt.Errorf("source account is not active")
		}

		destinationAcct, err := q.GetAccountByIDForUpdate(ctx, destAccountID)
		if err != nil {
			return fmt.Errorf("error locking destination account: %w", err)
		}

		systemAcct, err := q.GetAccountByExternalRef(ctx, "system_fee_revenue_ngn")
		if err != nil {
			return fmt.Errorf("error looking up system fee account: %w", err)
		}
		systemFeeAcct, err := q.GetAccountByIDForUpdate(ctx, systemAcct.ID)
		if err != nil {
			return fmt.Errorf("error locking system fee account: %w", err)
		}

		if sourceAcct.CurrencyCode != params.CurrencyCode || destinationAcct.CurrencyCode != params.CurrencyCode {
			return fmt.Errorf("currency code mismatch")
		}

		// Check Balance
		balance, err := internal.GetOrCreateBalanceProjection(ctx, q, sourceAcct.ID, sourceAcct.CurrencyCode, "available")
		if err != nil {
			return fmt.Errorf("getting source balance: %w", err)
		}

		amtDec, _ := decimal.NewFromString(internal.NumericToString(amount))
		feeDec, _ := decimal.NewFromString(internal.NumericToString(feeAmount))
		needed := amtDec.Add(feeDec)
		avail, _ := decimal.NewFromString(internal.NumericToString(balance.AvailableBalance))

		if avail.Cmp(needed) < 0 {
			return fmt.Errorf("insufficient funds")
		}

		destBalance, err := internal.GetOrCreateBalanceProjection(ctx, q, destinationAcct.ID, destinationAcct.CurrencyCode, "available")
		if err != nil {
			return fmt.Errorf("getting destination balance: %w", err)
		}

		// Create Request
		trf, err = q.CreateTransferRequest(ctx, db.CreateTransferRequestParams{
			IdempotencyKeyID:     idempkey.ID,
			CustomerID:           customer.ID,
			SourceAccountID:      sourceAcct.ID,
			DestinationAccountID: destinationAcct.ID,
			CurrencyCode:         params.CurrencyCode,
			Amount:               amount,
			FeeAmount:            feeAmount,
			ClientReference:      pgtype.Text{String: params.ClientReference, Valid: params.ClientReference != ""},
			ExternalReference:    pgtype.Text{String: params.ExternalReference, Valid: params.ExternalReference != ""},
			Metadata:             metaBytes,
		})
		if err != nil {
			return fmt.Errorf("creating transfer request: %w", err)
		}

		// Create JTX
		jtx, err = q.CreateJournalTransaction(ctx, db.CreateJournalTransactionParams{
			TransactionRef:    uuid.NewString(),
			TransferRequestID: trf.ID,
			IdempotencyKeyID:  idempkey.ID,
			Status:            "pending",
			EntryType:         "transfer",
			AccountingDate:    pgtype.Date{Time: trf.RequestedAt.Time, Valid: true},
			EffectiveAt:       pgtype.Timestamptz{Time: trf.RequestedAt.Time, Valid: true},
			SourceSystem:      params.Sourcesystem,
			SourceEventID:     pgtype.Text{String: trf.ID.String(), Valid: true},
			Description:       pgtype.Text{String: params.Description, Valid: params.Description != ""},
			Metadata:          metaBytes,
		})
		if err != nil {
			return fmt.Errorf("creating journal transaction: %w", err)
		}

		neededNumeric, _ := internal.StringToNumeric(needed.String())
		legs := []internal.JournalLeg{
			{AccountID: sourceAcct.ID, Amount: neededNumeric, Side: "debit"},
			{AccountID: destinationAcct.ID, Amount: amount, Side: "credit"},
		}
		if feeDec.IsPositive() {
			legs = append(legs, internal.JournalLeg{
				AccountID: systemFeeAcct.ID,
				Amount:    feeAmount,
				Side:      "credit",
			})
		}

		if err := internal.ValidateLedgerBalance(legs); err != nil {
			return fmt.Errorf("validating ledger balance: %w", err)
		}

		var sourceLineID, destLineID pgtype.UUID
		memo := pgtype.Text{String: params.Memo, Valid: params.Memo != ""}
		for i, leg := range legs {
			line, err := q.CreateJournalLine(ctx, db.CreateJournalLineParams{
				JournalTransactionID: jtx.ID,
				LineNumber:           int32(i) + 1,
				AccountID:            leg.AccountID,
				Side:                 leg.Side,
				Amount:               leg.Amount,
				CurrencyCode:         params.CurrencyCode,
				BalanceKind:          "available",
				Memo:                 memo,
				Metadata:             metaBytes,
			})
			if err != nil {
				return fmt.Errorf("creating journal line %d: %w", i+1, err)
			}
			if leg.AccountID == sourceAcct.ID && leg.Side == "debit" {
				sourceLineID = line.ID
			}
			if leg.AccountID == destinationAcct.ID && leg.Side == "credit" {
				destLineID = line.ID
			}
		}

		// Mark JTX Posted
		if _, err = q.MarkJournalTransactionPosted(ctx, jtx.ID); err != nil {
			return fmt.Errorf("marking transaction posted: %w", err)
		}

		// Update Balances
		srcLedger, _ := decimal.NewFromString(internal.NumericToString(balance.LedgerBalance))
		srcHeld, _ := decimal.NewFromString(internal.NumericToString(balance.HeldBalance))
		newSrcLedger := srcLedger.Sub(needed)
		newSrcAvail := newSrcLedger.Sub(srcHeld)

		nl, _ := internal.StringToNumeric(newSrcLedger.String())
		na, _ := internal.StringToNumeric(newSrcAvail.String())
		nh, _ := internal.StringToNumeric(srcHeld.String())

		err = q.UpsertBalanceProjectionWithExpectedVersion(ctx, db.UpsertBalanceProjectionWithExpectedVersionParams{
			AccountID:        sourceAcct.ID,
			CurrencyCode:     params.CurrencyCode,
			BalanceKind:      "available",
			LedgerBalance:    nl,
			AvailableBalance: na,
			HeldBalance:      nh,
			LastTxID:         jtx.ID,
			LastLineID:       sourceLineID,
			ExpectedVersion:  balance.Version,
		})
		if err != nil {
			return fmt.Errorf("updating source balance: %w", err)
		}

		dstLedger, _ := decimal.NewFromString(internal.NumericToString(destBalance.LedgerBalance))
		dstHeld, _ := decimal.NewFromString(internal.NumericToString(destBalance.HeldBalance))
		newDstLedger := dstLedger.Add(amtDec)
		newDstAvail := newDstLedger.Sub(dstHeld)

		ndl, _ := internal.StringToNumeric(newDstLedger.String())
		nda, _ := internal.StringToNumeric(newDstAvail.String())
		ndh, _ := internal.StringToNumeric(dstHeld.String())

		err = q.UpsertBalanceProjectionWithExpectedVersion(ctx, db.UpsertBalanceProjectionWithExpectedVersionParams{
			AccountID:        destinationAcct.ID,
			CurrencyCode:     params.CurrencyCode,
			BalanceKind:      "available",
			LedgerBalance:    ndl,
			AvailableBalance: nda,
			HeldBalance:      ndh,
			LastTxID:         jtx.ID,
			LastLineID:       destLineID,
			ExpectedVersion:  destBalance.Version,
		})
		if err != nil {
			return fmt.Errorf("updating destination balance: %w", err)
		}

		// Update Request Status
		trf, err = q.UpdateTransferRequestStatus(ctx, db.UpdateTransferRequestStatusParams{
			Status: "posted",
			ID:     trf.ID,
		})
		if err != nil {
			return fmt.Errorf("updating transfer status: %w", err)
		}

		// Create Outbox Event
		payloadBytes, _ := json.Marshal(trf)
		headersMap := map[string]interface{}{
			"content_type":      "application/json",
			"source_system":     params.Sourcesystem,
			"event_transaction": jtx.TransactionRef,
			"client_reference":  params.ClientReference,
		}
		headersBytes, _ := json.Marshal(headersMap)
		_, err = q.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
			AggregateType:    "transfer_request",
			AggregateID:      trf.ID,
			EventType:        "transfer.posted",
			IdempotencyKeyID: idempkey.ID,
			Payload:          payloadBytes,
			Headers:          headersBytes,
			PartitionKey:     pgtype.Text{String: trf.DestinationAccountID.String(), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("creating outbox event: %w", err)
		}

		// Save Idempotency
		return internal.SaveIdem(ctx, q, rdb, params.CustomerID, params.IdempotencyKeyID, idempkey.ID, payloadBytes, 201)
	})

	if err != nil {
		return nil, err
	}

	resp := internal.ToTransferResponse(trf, &jtx)
	return &resp, nil
}

func GetTransferByID(ctx context.Context, transferID string, queries *db.Queries) (*internal.TransferResponse, error) {
	id, err := internal.StringtoPgUuid(transferID)
	if err != nil {
		return nil, fmt.Errorf("invalid transfer ID format: %w", err)
	}

	transfer, err := queries.GetTransferRequestByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Try to find journal transaction
	jtx, err := queries.GetJournalTransactionByRef(ctx, transferID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("error looking up journal transaction: %w", err)
	}

	var resp internal.TransferResponse
	if errors.Is(err, pgx.ErrNoRows) {
		resp = internal.ToTransferResponse(transfer, nil)
	} else {
		resp = internal.ToTransferResponse(transfer, &jtx)
	}

	return &resp, nil
}
