package repositry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database"
	db "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/internal/utilities"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

func HandleTransferFailed(ctx context.Context, data json.RawMessage, d *database.Db, rdb *redis.Client) error {
	var transferData internal.WebhookTransferData

	requestHash := internal.HashRequest(data)
	json.Unmarshal(data, &transferData)

	trfID, _ := internal.StringtoPgUuid(transferData.Reference)

	check, err := internal.IdemCheck(ctx, d.Queries, rdb, transferData.Reference, transferData.Status, requestHash, "transfer_success")
	if err != nil {
		return fmt.Errorf("Error checking idempotency key: %v", err)
	}
	if !check.ShouldProceed {
		return nil
	}
	idempkey, err := d.Queries.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
		IdempotencyKey: transferData.Reference,
		Scope:          "Transfer_failed",
		RequestHash:    []byte(requestHash),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(24 * time.Hour),
			Valid: true,
		},
	})
	if err != nil {
		return fmt.Errorf("Error creating idempotency key: %v", err)
	}

	err = d.ExecTx(ctx, func(q *db.Queries) error {

		trf, err := q.GetTransferRequestByIDForUpdate(ctx, trfID)
		if err != nil {
			return fmt.Errorf("Error getting Transfer by ID %v", err)
		}
		if trf.Status == "failed" {
			return nil
		}

		hold, err := q.GetActiveHoldByTransferRequestID(ctx, trf.ID)
		if err != nil {
			return fmt.Errorf("Error getting Hold: %v", err)
		}
		// Release hold — funds return to available
		_, err = q.ReleaseHold(ctx, db.ReleaseHoldParams{
			ID:     hold.ID,
			Amount: trf.Amount,
		})
		if err != nil {
			return fmt.Errorf("Error releasing Hold: %v", err)
		}

		balance, err := internal.GetOrCreateBalanceProjection(
			ctx, q,
			trf.SourceAccountID,
			trf.CurrencyCode,
			"available",
		)
		if err != nil {
			return fmt.Errorf("getting balance: %w", err)
		}
		// Balance: available ↑, held ↓, ledger unchanged
		heldDecimal, _ := decimal.NewFromString(internal.NumericToString(balance.HeldBalance))
		ledgerDecimal, _ := decimal.NewFromString(internal.NumericToString(balance.LedgerBalance))
		amt, _ := decimal.NewFromString(internal.NumericToString(trf.Amount))

		newHeld := heldDecimal.Sub(amt)
		newAvail := ledgerDecimal.Sub(newHeld)

		newHeldNum, _ := internal.StringToNumeric(newHeld.String())
		newAvailNum, _ := internal.StringToNumeric(newAvail.String())
		ledgerNum, _ := internal.StringToNumeric(ledgerDecimal.String())

		err = q.UpsertBalanceProjectionWithExpectedVersion(ctx,
			db.UpsertBalanceProjectionWithExpectedVersionParams{
				AccountID:        trf.SourceAccountID,
				CurrencyCode:     trf.CurrencyCode,
				BalanceKind:      "available",
				LedgerBalance:    ledgerNum,
				AvailableBalance: newAvailNum,
				HeldBalance:      newHeldNum,
				LastTxID:         hold.JournalTransactionID,
				ExpectedVersion:  balance.Version,
			})
		if err != nil {
			return fmt.Errorf("Error Upserting Balance with Expected Version: %v", err)
		}

		trf, err = q.UpdateTransferRequestStatus(ctx, db.UpdateTransferRequestStatusParams{
			ID:     trf.ID,
			Status: "failed",
		})
		if err != nil {
			return fmt.Errorf("Error updating transfer request: %v", err)
		}

		transferPayload := internal.ToTransferResponse(trf, nil)
		payloadBytes, err := json.Marshal(transferPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal outbox payload: %w", err)
		}
		if err = internal.SaveIdem(ctx, d.Queries, rdb, transferData.Status, transferData.Reference, idempkey.ID, payloadBytes, 400); err != nil {
			return fmt.Errorf("error saving idempotency key: %v", err)
		}
		headersMap := map[string]interface{}{
			"content_type":      "application/json",
			"source_system":     transferData.ID,
			"event_transaction": trf.ID.String(),
			"client_reference":  transferData.Reference,
		}
		headersBytes, err := json.Marshal(headersMap)
		if err != nil {
			return fmt.Errorf("failed to marshal outbox headers: %w", err)
		}
		_, err = q.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
			AggregateType:    "transfer_request",
			AggregateID:      trf.ID,
			EventType:        "transfer.failed",
			IdempotencyKeyID: trf.IdempotencyKeyID,
			Payload:          payloadBytes,
			Headers:          headersBytes,
			PartitionKey:     pgtype.Text{String: trf.SourceAccountID.String(), Valid: true},
		})
		return err
	})
	return nil
}

func HandleTransferSuccess(ctx context.Context, data json.RawMessage, d *database.Db, rdb *redis.Client) error {
	var transferData internal.WebhookTransferData
	requestHash := internal.HashRequest(data)
	if err := json.Unmarshal(data, &transferData); err != nil {
		return fmt.Errorf("invalid transfer data: %s", err)
	}

	// Reference is the transfer_request_id you sent Paystack
	trfID, err := internal.StringtoPgUuid(transferData.Reference)
	if err != nil {
		return fmt.Errorf("Invalid reference")
	}

	// zero, _ := StringToNumeric("0.00")
	check, err := internal.IdemCheck(ctx, d.Queries, rdb, transferData.Reference, transferData.Status, requestHash, transferData.Status)
	if err != nil {
		return fmt.Errorf("Error checking idempotency key: %v", err)
	}
	if !check.ShouldProceed {
		return nil
	}

	idempkey, err := d.Queries.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
		IdempotencyKey: transferData.Reference,
		Scope:          "transfer_success",
		RequestHash:    []byte(requestHash),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(24 * time.Hour),
			Valid: true,
		},
	})
	if err != nil {
		return fmt.Errorf("Error creating idempotency key: %v", err)
	}

	err = d.ExecTx(ctx, func(q *db.Queries) error {

		trf, err := q.GetTransferRequestByIDForUpdate(ctx, trfID)
		if err != nil {
			return fmt.Errorf("transfer not found: %w", err)
		}
		if trf.Status == "posted" {
			return fmt.Errorf("Transferr already processed")
		}
		Meta, err := json.Marshal(map[string]string{"ID": transferData.ID, "status": transferData.Status, "reference": transferData.Reference})
		if err != nil {
			return fmt.Errorf("Unable to parse Metadata payload: %w", err)
		}

		// 2. Get the active hold for this transfer
		hold, err := q.GetActiveHoldByTransferRequestID(ctx, trf.ID)
		if err != nil {
			return fmt.Errorf("Error Getting Hold: %w", err)
		}

		// 3. Lock balance projection
		balance, err := internal.GetOrCreateBalanceProjection(
			ctx, q,
			trf.SourceAccountID,
			trf.CurrencyCode,
			"available",
		)
		if err != nil {
			return fmt.Errorf("Error getting balance: %w", err)
		}

		// 4. Create the SETTLEMENT journal transaction — the real debit
		jtx, err := q.CreateJournalTransaction(ctx, db.CreateJournalTransactionParams{
			TransactionRef:    uuid.NewString(),
			TransferRequestID: trf.ID,
			IdempotencyKeyID:  idempkey.ID,
			Status:            "pending",
			EntryType:         "withdrawal",
			AccountingDate:    pgtype.Date{Time: time.Now(), Valid: true},
			EffectiveAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
			SourceSystem:      "paystack_webhook",
			SourceEventID:     pgtype.Text{String: transferData.ID, Valid: true},
			Metadata:          Meta,
		})
		if err != nil {
			return fmt.Errorf("Error Creating Journal transaction %v", err)
		}

		settlementAcct, err := q.GetAccountByExternalRef(ctx, "system_settlement_ngn")
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("System settlement account not found")
			}
			return fmt.Errorf("Error looking up system settlement account: %v", err)
		}

		// 5. Journal lines — actual debit of customer, credit settlement
		_, err = q.CreateJournalLine(ctx, db.CreateJournalLineParams{
			JournalTransactionID: jtx.ID,
			LineNumber:           1,
			AccountID:            trf.SourceAccountID,
			Side:                 "debit",
			Amount:               trf.Amount,
			CurrencyCode:         trf.CurrencyCode,
			BalanceKind:          "ledger",
			Memo:                 pgtype.Text{String: "withdrawal settled", Valid: true},
			Metadata:             Meta,
		})
		if err != nil {
			return fmt.Errorf("Error creating journal line: %v", err)
		}

		journalLine, err := q.CreateJournalLine(ctx, db.CreateJournalLineParams{
			JournalTransactionID: jtx.ID,
			LineNumber:           2,
			AccountID:            settlementAcct.ID,
			Side:                 "credit",
			Amount:               trf.Amount,
			CurrencyCode:         trf.CurrencyCode,
			BalanceKind:          "ledger",
			Memo:                 pgtype.Text{String: "withdrawal settled", Valid: true},
			Metadata:             Meta,
		})
		if err != nil {
			return fmt.Errorf("Error creating journal line: %v", err)
		}

		// 6. Consume the hold
		_, err = q.ConsumeHold(ctx, db.ConsumeHoldParams{
			ID:     hold.ID,
			Amount: trf.Amount,
		})
		if err != nil {
			return fmt.Errorf("Error consuming hold: %v", err)
		}

		// 7. Mark journal transaction posted
		_, err = q.MarkJournalTransactionPosted(ctx, jtx.ID)
		if err != nil {
			return fmt.Errorf("Error marking Journal: %v", err)
		}

		// 8. Update balance projection — held ↓, ledger ↓, available unchanged
		heldDecimal, _ := decimal.NewFromString(internal.NumericToString(balance.HeldBalance))
		ledgerDecimal, _ := decimal.NewFromString(internal.NumericToString(balance.LedgerBalance))
		amt, _ := decimal.NewFromString(internal.NumericToString(trf.Amount))

		newLedger := ledgerDecimal.Sub(amt)
		newHeld := heldDecimal.Sub(amt)
		newAvail := newLedger.Sub(newHeld)

		newLedgerNum, _ := internal.StringToNumeric(newLedger.String())
		newHeldNum, _ := internal.StringToNumeric(newHeld.String())
		newAvailNum, _ := internal.StringToNumeric(newAvail.String())

		err = q.UpsertBalanceProjectionWithExpectedVersion(ctx,
			db.UpsertBalanceProjectionWithExpectedVersionParams{
				AccountID:        trf.SourceAccountID,
				CurrencyCode:     trf.CurrencyCode,
				BalanceKind:      "available",
				LedgerBalance:    newLedgerNum,
				AvailableBalance: newAvailNum,
				HeldBalance:      newHeldNum,
				LastTxID:         pgtype.UUID{Bytes: jtx.ID.Bytes, Valid: true},
				LastLineID:       journalLine.ID,
				ExpectedVersion:  balance.Version,
			})
		if err != nil {
			return fmt.Errorf("upserting source balance: %w", err)
		}

		// 9. Update transfer request → posted
		_, err = q.UpdateTransferRequestStatus(ctx, db.UpdateTransferRequestStatusParams{
			ID:     trf.ID,
			Status: "posted",
		})
		if err != nil {
			return fmt.Errorf("updating transfer request status: %w", err)
		}
		transferPayload := internal.ToTransferResponse(trf, &jtx)
		payloadBytes, err := json.Marshal(transferPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		if err = internal.SaveIdem(ctx, d.Queries, rdb, transferData.Status, transferData.Reference, idempkey.ID, payloadBytes, 201); err != nil {
			return fmt.Errorf("error saving idempotency key: %v", err)
		}
		headersMap := map[string]interface{}{
			"content_type":      "application/json",
			"source_system":     transferData.ID,
			"event_transaction": jtx.TransactionRef,
			"client_reference":  transferData.Reference,
		}
		headersBytes, err := json.Marshal(headersMap)
		if err != nil {
			return fmt.Errorf("failed to marshal outbox headers: %w", err)
		}

		// 10. Outbox event — client gets notified via this
		_, err = q.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
			AggregateType:    "transfer_request",
			AggregateID:      trf.ID,
			EventType:        "transfer.posted",
			IdempotencyKeyID: trf.IdempotencyKeyID,
			Payload:          payloadBytes,
			Headers:          headersBytes,
			PartitionKey:     pgtype.Text{String: trf.SourceAccountID.String(), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("Error creating Outboxevent: %w", err)
		}
		return nil
	})
	return nil
}
