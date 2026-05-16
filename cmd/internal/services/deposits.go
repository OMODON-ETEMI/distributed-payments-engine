package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

func DepositLogic(ctx context.Context, payload []byte, d *database.Db, rdb *redis.Client) (*internal.TransferResponse, error) {
	var params internal.DepositParams
	if err := json.Unmarshal(payload, &params); err != nil {
		log.Printf("Error parsing payload: %v", err)
		return nil, err
	}
	requestHash := internal.HashRequest(payload)
	customerID, err := internal.StringtoPgUuid(params.CustomerID)
	if err != nil {
		log.Printf("Error parsing customer ID: %v", err)
		return nil, fmt.Errorf("Error parsing customer ID: %v", err)
	}
	customer, err := d.Queries.GetCustomerByID(ctx, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("customer not found")
		}
		return nil, fmt.Errorf("Error looking up customer: %v", err)
	}
	if customer.Status != "active" {
		return nil, fmt.Errorf("customer is not active")
	}
	amount, err := internal.StringToNumeric(params.Amount)
	if err != nil {
		return nil, fmt.Errorf("Error parsing amount: %v", err)
	}
	feeAmount, err := internal.StringToNumeric(params.FeeAmount)
	if err != nil {
		return nil, fmt.Errorf("Error parsing Fee amount: %v", err)
	}
	ClientReference := pgtype.Text{String: params.ClientReference, Valid: params.ClientReference != ""}
	ExternalReference := pgtype.Text{String: params.ExternalReference, Valid: params.ExternalReference != ""}
	metaBytes := []byte("null")
	if params.Metadata != nil {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			return nil, fmt.Errorf("Error parsing metadata: %v", err)
		}
		metaBytes = b
	}
	memo := pgtype.Text{String: params.Memo, Valid: params.Memo != ""}
	check, err := internal.IdemCheck(ctx, d.Queries, rdb, params.IdempotencyKeyID, params.CustomerID, requestHash, "deposit_create")
	if err != nil {
		return nil, fmt.Errorf("Error checking idempotency key: %v", err)
	}
	if !check.ShouldProceed {
		var cachedData internal.TransferResponse
		err := json.Unmarshal(check.CachedResponse, &cachedData)
		if err != nil {
			fmt.Printf("Failed to unmarshal cached response: %v", err)
			return nil, fmt.Errorf("Failed to unmarshal cached response: %v", err)
		}
		return &cachedData, nil
	}
	idempkey, err := d.Queries.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
		IdempotencyKey: params.IdempotencyKeyID,
		Scope:          "deposit_create",
		RequestHash:    []byte(requestHash),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(24 * time.Hour),
			Valid: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating idempotency key: %v", err)
	}

	var trf db.TransferRequest
	var jtx db.JournalTransaction
	var payloadBytes []byte
	err = d.ExecTx(ctx, func(q *db.Queries) error {
		systemSettleAcct, err := q.GetAccountByExternalRef(ctx, "system_settlement_ngn")
		if err != nil {
			return fmt.Errorf("Error looking up system settlement account: %v", err)
		}

		destUuid, _ := internal.StringtoPgUuid(params.DestinationAccountID)
		destAcct, err := q.GetAccountByIDForUpdate(ctx, destUuid)
		if err != nil {
			return fmt.Errorf("Error looking up destination account: %v", err)
		}

		if destAcct.Status != "active" {
			return fmt.Errorf("destination account is not active")
		}
		if params.CurrencyCode != destAcct.CurrencyCode {
			return fmt.Errorf("currency code mismatch")
		}

		trf, err = q.CreateTransferRequest(ctx, db.CreateTransferRequestParams{
			IdempotencyKeyID:     idempkey.ID,
			CustomerID:           customer.ID,
			SourceAccountID:      systemSettleAcct.ID,
			DestinationAccountID: destAcct.ID,
			CurrencyCode:         params.CurrencyCode,
			Amount:               amount,
			FeeAmount:            feeAmount,
			ClientReference:      ClientReference,
			ExternalReference:    ExternalReference,
			Metadata:             metaBytes,
		})
		if err != nil {
			return fmt.Errorf("Error creating transfer request for deposit: %v", err)
		}

		jtx, err = q.CreateJournalTransaction(ctx, db.CreateJournalTransactionParams{
			TransactionRef:    uuid.NewString(),
			TransferRequestID: trf.ID,
			IdempotencyKeyID:  idempkey.ID,
			Status:            "pending",
			EntryType:         "deposit",
			AccountingDate:    pgtype.Date{Time: trf.RequestedAt.Time, Valid: true},
			EffectiveAt:       pgtype.Timestamptz{Time: trf.RequestedAt.Time, Valid: true},
			SourceSystem:      params.Sourcesystem,
			SourceEventID:     pgtype.Text{String: trf.ID.String(), Valid: true},
			Description:       pgtype.Text{String: params.Description, Valid: params.Description != ""},
			Metadata:          metaBytes,
		})
		if err != nil {
			return fmt.Errorf("Error creating journal transaction: %v", err)
		}
		legs := []internal.JournalLeg{
			{AccountID: systemSettleAcct.ID, Amount: amount, Side: "debit"},
			{AccountID: destAcct.ID, Amount: amount, Side: "credit"},
		}
		err = internal.ValidateLedgerBalance(legs)
		if err != nil {
			return err
		}
		var destLineID pgtype.UUID
		for index, leg := range legs {
			line, err := q.CreateJournalLine(ctx, db.CreateJournalLineParams{
				JournalTransactionID: jtx.ID,
				LineNumber:           int32(index) + 1,
				AccountID:            leg.AccountID,
				Side:                 leg.Side,
				Amount:               leg.Amount,
				CurrencyCode:         params.CurrencyCode,
				BalanceKind:          "available",
				Memo:                 memo,
				Metadata:             metaBytes,
			})
			if err != nil {
				return fmt.Errorf("Error creating journal line: %v", err)
			}
			if leg.AccountID == destAcct.ID && leg.Side == "credit" {
				destLineID = line.ID
			}
		}
		_, err = q.MarkJournalTransactionPosted(ctx, jtx.ID)
		if err != nil {
			return fmt.Errorf("Error Marking journal transaction: %v", err)
		}
		balance, err := internal.GetOrCreateBalanceProjection(
			ctx, q,
			destAcct.ID,
			params.CurrencyCode,
			"available",
		)
		if err != nil {
			return fmt.Errorf("getting balance: %w", err)
		}
		destLedger, _ := decimal.NewFromString(internal.NumericToString(balance.LedgerBalance))
		destHeld, _ := decimal.NewFromString(internal.NumericToString(balance.HeldBalance))
		amt, _ := decimal.NewFromString(internal.NumericToString(amount))
		newDestLedger := destLedger.Add(amt)
		newDestHeld := destHeld
		newDestAvail := newDestLedger.Sub(newDestHeld)

		newDestLedgerNum, err := internal.StringToNumeric(newDestLedger.String())
		newDestAvailNum, err := internal.StringToNumeric(newDestAvail.String())
		newDestHeldNum, err := internal.StringToNumeric(newDestHeld.String())
		err = q.UpsertBalanceProjectionWithExpectedVersion(ctx, db.UpsertBalanceProjectionWithExpectedVersionParams{
			AccountID:        destAcct.ID,
			CurrencyCode:     params.CurrencyCode,
			BalanceKind:      "available",
			LedgerBalance:    newDestLedgerNum,
			AvailableBalance: newDestAvailNum,
			HeldBalance:      newDestHeldNum,
			LastTxID:         jtx.ID,
			LastLineID:       destLineID,
			ExpectedVersion:  balance.Version,
		})
		if err != nil {
			return fmt.Errorf("upserting destination balance: %w", err)
		}
		trf, err = q.UpdateTransferRequestStatus(ctx, db.UpdateTransferRequestStatusParams{
			Status: "posted",
			ID:     trf.ID,
		})
		if err != nil {
			return fmt.Errorf("Error Updating the transfer reuest status : %w", err)
		}
		transferPayload := internal.ToTransferResponse(trf, &jtx)
		payloadBytes, err = json.Marshal(transferPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal outbox payload: %w", err)
		}
		headersMap := map[string]interface{}{
			"content_type":      "application/json",
			"source_system":     params.Sourcesystem,
			"event_transaction": jtx.TransactionRef,
			"client_reference":  params.ClientReference,
		}
		headersBytes, err := json.Marshal(headersMap)
		if err != nil {
			return fmt.Errorf("failed to marshal outbox headers: %w", err)
		}
		partitionKey := pgtype.Text{String: trf.DestinationAccountID.String(), Valid: true}
		_, err = q.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
			AggregateType:    "deposit_request",
			AggregateID:      trf.ID,
			EventType:        "transfer.posted",
			IdempotencyKeyID: idempkey.ID,
			Payload:          payloadBytes,
			Headers:          headersBytes,
			PartitionKey:     partitionKey,
		})
		if err != nil {
			return fmt.Errorf("failed to create outbox event: %w", err)
		}
		if err := internal.SaveIdem(ctx, d.Queries, rdb, params.CustomerID, params.IdempotencyKeyID, idempkey.ID, payloadBytes, 201); err != nil {
			return fmt.Errorf("error saving idempotency key: %v", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("transfer failed: %v", err)
	}
	response := internal.ToTransferResponse(trf, &jtx)
	return &response, nil
}
