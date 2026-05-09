package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

type WithddrawParams struct {
	IdempotencyKeyID     string                 `json:"idempotency_key_id"`
	CustomerID           string                 `json:"customer_id"`
	SourceAccountID      string                 `json:"source_account_id"`
	DestinationAccountID string                 `json:"destination_account_id"`
	CurrencyCode         string                 `json:"currency_code"`
	Sourcesystem         string                 `json:"source_system"`
	Description          string                 `json:"description"`
	Amount               string                 `json:"amount"`
	FeeAmount            string                 `json:"fee_amount"`
	ClientReference      string                 `json:"client_reference"`
	ExternalReference    string                 `json:"external_reference"`
	Memo                 string                 `json:"memo"`
	Metadata             map[string]interface{} `json:"metadata"`
}

func (api *ApiConfig) HandleWithdraw(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error reading request body: %v", err))
		return
	}
	requestHash := HashRequest(bodyBytes)

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	decoder := json.NewDecoder(r.Body)
	params := WithddrawParams{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.IdempotencyKeyID == "" || params.CustomerID == "" || params.SourceAccountID == "" || params.CurrencyCode == "" || params.Description == "" {
		respondWithError(w, 400, "missing required fields: idempotency_key_id, customer_id, source_account_id, currency_code, description")
		return
	}
	if params.Metadata == nil {
		params.Metadata = make(map[string]interface{})
	}
	customerID, err := StringtoPgUuid(params.CustomerID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing customer ID: %v", err))
		return
	}
	customer, err := api.Db.Queries.GetCustomerByID(r.Context(), customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "customer not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up customer: %v", err))
		return
	}
	sourceAccountID, err := StringtoPgUuid(params.SourceAccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Source account ID: %v", err))
		return
	}
	sourceAccount, err := api.Db.Queries.GetAccountByID(r.Context(), sourceAccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "source account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up source account: %v", err))
		return
	}
	if sourceAccount.Status != "active" {
		respondWithError(w, 400, "source account is not active")
		return
	}
	if params.CurrencyCode != sourceAccount.CurrencyCode {
		respondWithError(w, 400, "currency code mismatch")
		return
	}
	amount, err := StringToNumeric(params.Amount)
	feeAmount, err := StringToNumeric(params.FeeAmount)
	metaBytes := []byte("null")
	if params.Metadata != nil {
		b, err := json.Marshal(params.Metadata)
		if err != nil {
			respondWithError(w, 400, fmt.Sprintf("Error parsing metadata: %v", err))
			return
		}
		metaBytes = b
	}
	check, err := api.IdemCheck(r.Context(), params.IdempotencyKeyID, params.CustomerID, requestHash, "withdraw_create")
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error checking idempotency key: %v", err))
		return
	}
	if !check.ShouldProceed {
		var cachedData TransferResponse
		err := json.Unmarshal(check.CachedResponse, &cachedData)
		if err != nil {
			fmt.Printf("Failed to unmarshal cached response: %v", err)
			respondeWithJson(w, check.StatusCode, string(check.CachedResponse))
			return
		}
		respondeWithJson(w, check.StatusCode, cachedData)
		return
	}
	idempkey, err := api.Db.Queries.CreateIdempotencyKey(r.Context(), db.CreateIdempotencyKeyParams{
		IdempotencyKey: params.IdempotencyKeyID,
		Scope:          "withdraw_create",
		RequestHash:    []byte(requestHash),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(24 * time.Hour),
			Valid: true,
		},
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error creating idempotency key: %v", err))
		return
	}

	var trf db.TransferRequest
	err = api.Db.ExecTx(r.Context(), func(q *db.Queries) error {
		SourceAcct, err := q.GetAccountByIDForUpdate(r.Context(), sourceAccount.ID)
		if err != nil {
			return fmt.Errorf("Error looking up source account: %v", err)
		}
		if SourceAcct.Status != "active" {
			return fmt.Errorf("account is not active")
		}
		settlement, err := q.GetAccountByExternalRef(r.Context(), "system_settlement_ngn")
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("System settlement account not found")
			}
			return fmt.Errorf("Error looking up system settlement account: %v", err)
		}
		balance, err := GetOrCreateBalanceProjection(r.Context(), q, SourceAcct.ID, params.CurrencyCode, "available")
		if err != nil {
			return fmt.Errorf("getting balance: %w", err)
		}
		avail, _ := decimal.NewFromString(NumericToString(balance.AvailableBalance))
		amt, _ := decimal.NewFromString(NumericToString(amount))
		if avail.LessThan(amt) {
			return fmt.Errorf("insufficient funds")
		}
		trf, err = q.CreateTransferRequest(r.Context(), db.CreateTransferRequestParams{
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
			return fmt.Errorf("Error creating transfer request for withdrawal: %w", err)
		}

		jtx, err := q.CreateJournalTransaction(r.Context(), db.CreateJournalTransactionParams{
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
			return fmt.Errorf("Error creating journal transaction: %v", err)
		}
		legs := []JournalLeg{
			{AccountID: sourceAccount.ID, BalanceKind: "available", Amount: amount, Side: "debit"},
			{AccountID: sourceAccount.ID, BalanceKind: "held", Amount: amount, Side: "credit"},
		}
		err = ValidateLedgerBalance(legs)
		if err != nil {
			return err
		}
		var sourceLineID pgtype.UUID
		for index, leg := range legs {
			line, err := q.CreateJournalLine(r.Context(), db.CreateJournalLineParams{
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
				return fmt.Errorf("Error creating journal line: %v", err)
			}
			if leg.AccountID == sourceAccount.ID && leg.Side == "debit" {
				sourceLineID = line.ID
			}
		}
		hold, err := q.CreateHold(r.Context(), db.CreateHoldParams{
			AccountID:            sourceAccount.ID,
			TransferRequestID:    trf.ID,
			JournalTransactionID: jtx.ID,
			IdempotencyKeyID:     idempkey.ID,
			Status:               "active",
			CurrencyCode:         params.CurrencyCode,
			Amount:               amount,
			RemainingAmount:      amount,
			ReleasedAmount: pgtype.Numeric{
				Int:   big.NewInt(0),
				Exp:   0,
				Valid: true,
			},
			CapturedAmount: pgtype.Numeric{
				Int:   big.NewInt(0),
				Exp:   0,
				Valid: true,
			},
			ReasonCode: pgtype.Text{String: "external_withdrawal", Valid: true},
			Reason:     pgtype.Text{String: params.Description, Valid: true},
			ExpiresAt: pgtype.Timestamptz{
				Time:  time.Now().Add(24 * time.Hour),
				Valid: true,
			},
		})
		destLedger, _ := decimal.NewFromString(NumericToString(balance.LedgerBalance))
		destHeld, _ := decimal.NewFromString(NumericToString(balance.HeldBalance))
		newDestLedger := destLedger
		newDestHeld := destHeld.Add(amt)
		newDestAvail := newDestLedger.Sub(amt)

		newDestLedgerNum, err := StringToNumeric(newDestLedger.String())
		newDestAvailNum, err := StringToNumeric(newDestAvail.String())
		newDestHeldNum, err := StringToNumeric(newDestHeld.String())
		err = q.UpsertBalanceProjectionWithExpectedVersion(r.Context(), db.UpsertBalanceProjectionWithExpectedVersionParams{
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
		payloadBytes, err := json.Marshal(map[string]string{"status": "pending", "hold_id": hold.ID.String()})
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		if err := api.saveIdem(r.Context(), params.CustomerID, params.IdempotencyKeyID, idempkey.ID, payloadBytes, 202); err != nil {
			return fmt.Errorf("error saving idempotency key: %w", err)
		}
		return nil
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("withdraw failed: %v", err))
		return
	}

	// ── Call Payment Provider ───────────────────────────────────────────

	providerResp, err := api.Router.Route(r.Context(), InitiateRequest{
		Amount:        params.Amount,
		Currency:      params.CurrencyCode,
		RecipientCode: params.DestinationAccountID,
		Reference:     trf.ID.String(),
		Reason:        params.Description,
	})
	if err != nil {
		jsonData, _ := json.Marshal(WebhookTransferData{
			ID:            trf.CustomerID.String(),
			Reference:     trf.ID.String(),
			Status:        "failed",
			FailureReason: err.Error(),
		})
		api.handleTransferFailed(r.Context(), json.RawMessage(jsonData), jsonData)
		return
	}

	respondeWithJson(w, 201, struct {
		Transfer         TransferResponse  `json:"transfer"`
		ProviderResponse *InitiateResponse `json:"provider_response"`
		Message          string            `json:"message"`
	}{
		Transfer:         ToTransferResponse(trf, nil),
		ProviderResponse: providerResp,
		Message:          "withdrawal initiated, processing",
	})
}

func (api *ApiConfig) handleTransferFailed(ctx context.Context, data json.RawMessage, rawBody []byte) error {
	var transferData WebhookTransferData

	requestHash := HashRequest(rawBody)
	json.Unmarshal(data, &transferData)

	trfID, _ := StringtoPgUuid(transferData.Reference)

	check, err := api.IdemCheck(ctx, transferData.Reference, transferData.Status, requestHash, "transfer_success")
	if err != nil {
		return fmt.Errorf("Error checking idempotency key: %v", err)
	}
	if !check.ShouldProceed {
		return nil
	}
	idempkey, err := api.Db.Queries.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
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

	err = api.Db.ExecTx(ctx, func(q *db.Queries) error {

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

		balance, err := GetOrCreateBalanceProjection(
			ctx, q,
			trf.SourceAccountID,
			trf.CurrencyCode,
			"available",
		)
		if err != nil {
			return fmt.Errorf("getting balance: %w", err)
		}
		// Balance: available ↑, held ↓, ledger unchanged
		heldDecimal, _ := decimal.NewFromString(NumericToString(balance.HeldBalance))
		ledgerDecimal, _ := decimal.NewFromString(NumericToString(balance.LedgerBalance))
		amt, _ := decimal.NewFromString(NumericToString(trf.Amount))

		newHeld := heldDecimal.Sub(amt)
		newAvail := ledgerDecimal.Sub(newHeld)

		newHeldNum, _ := StringToNumeric(newHeld.String())
		newAvailNum, _ := StringToNumeric(newAvail.String())
		ledgerNum, _ := StringToNumeric(ledgerDecimal.String())

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

		transferPayload := ToTransferResponse(trf, nil)
		payloadBytes, err := json.Marshal(transferPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal outbox payload: %w", err)
		}
		if err = api.saveIdem(ctx, transferData.Status, transferData.Reference, idempkey.ID, payloadBytes, 400); err != nil {
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
			EventType:        "transfer.posted",
			IdempotencyKeyID: trf.IdempotencyKeyID,
			Payload:          payloadBytes,
			Headers:          headersBytes,
			PartitionKey:     pgtype.Text{String: trf.SourceAccountID.String(), Valid: true},
		})
		return fmt.Errorf("Error creating Outboxevent: %v", err)
	})
	return nil
}

func (api *ApiConfig) handleTransferSuccess(ctx context.Context, data json.RawMessage, rawBody []byte) error {
	var transferData WebhookTransferData
	requestHash := HashRequest(rawBody)
	if err := json.Unmarshal(data, &transferData); err != nil {
		return fmt.Errorf("invalid transfer data: %s", err)
	}

	// Reference is the transfer_request_id you sent Paystack
	trfID, err := StringtoPgUuid(transferData.Reference)
	if err != nil {
		return fmt.Errorf("Invalid reference")
	}

	// zero, _ := StringToNumeric("0.00")
	check, err := api.IdemCheck(ctx, transferData.Reference, transferData.Status, requestHash, transferData.Status)
	if err != nil {
		return fmt.Errorf("Error checking idempotency key: %v", err)
	}
	if !check.ShouldProceed {
		return nil
	}

	idempkey, err := api.Db.Queries.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
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

	err = api.Db.ExecTx(ctx, func(q *db.Queries) error {

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
		balance, err := GetOrCreateBalanceProjection(
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
		heldDecimal, _ := decimal.NewFromString(NumericToString(balance.HeldBalance))
		ledgerDecimal, _ := decimal.NewFromString(NumericToString(balance.LedgerBalance))
		amt, _ := decimal.NewFromString(NumericToString(trf.Amount))

		newLedger := ledgerDecimal.Sub(amt)
		newHeld := heldDecimal.Sub(amt)
		newAvail := newLedger.Sub(newHeld)

		newLedgerNum, _ := StringToNumeric(newLedger.String())
		newHeldNum, _ := StringToNumeric(newHeld.String())
		newAvailNum, _ := StringToNumeric(newAvail.String())

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
		transferPayload := ToTransferResponse(trf, &jtx)
		payloadBytes, err := json.Marshal(transferPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		if err = api.saveIdem(ctx, transferData.Status, transferData.Reference, idempkey.ID, payloadBytes, 201); err != nil {
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
