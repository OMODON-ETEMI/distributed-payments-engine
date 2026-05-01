package routes

import (
	"bytes"
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
	if params.IdempotencyKeyID == "" || params.CustomerID == "" || params.SourceAccountID == "" || params.DestinationAccountID == "" || params.CurrencyCode == "" || params.Description == "" {
		respondWithError(w, 400, "missing required fields: idempotency_key_id, customer_id, source_account_id, destination_account_id, currency_code, description")
		return
	}
	customerID, err := StringtoPgUuid(params.CustomerID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing customer ID: %v", err))
		return
	}
	idempotencyKey, err := StringtoPgUuid(params.IdempotencyKeyID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Idempotency KeyID: %v", err))
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
	var trf db.TransferRequest
	err = api.Db.ExecTx(r.Context(), func(q *db.Queries) error {
		check, err := api.IdemCheck(r.Context(), params.IdempotencyKeyID, params.CustomerID, requestHash, "withdraw_create")
		if err != nil {
			return fmt.Errorf("Error checking idempotency key: %v", err)
		}
		if !check.ShouldProceed {
			return fmt.Errorf("idem response %v, status code %v ", check.CachedResponse, check.StatusCode)
		}
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
		balance, err := q.GetBalanceProjectionForUpdate(r.Context(), db.GetBalanceProjectionForUpdateParams{
			AccountID:    SourceAcct.ID,
			CurrencyCode: SourceAcct.CurrencyCode,
			BalanceKind:  "available",
		})
		if err != nil {
			return fmt.Errorf("Error looking up balance: %v", err)
		}
		avail, _ := decimal.NewFromString(NumericToString(balance.AvailableBalance))
		amt, _ := decimal.NewFromString(NumericToString(amount))
		if avail.LessThan(amt) {
			return fmt.Errorf("insufficient funds")
		}
		trf, err := q.CreateTransferRequest(r.Context(), db.CreateTransferRequestParams{
			IdempotencyKeyID:     idempotencyKey,
			CustomerID:           customer.ID,
			SourceAccountID:      sourceAccount.ID,
			DestinationAccountID: settlement.ID,
			CurrencyCode:         params.CurrencyCode,
			Amount:               amount,
			FeeAmount:            feeAmount,
			Metadata:             metaBytes,
		})
		if err != nil {
			return fmt.Errorf("Error creating transfer request: %w", err)
		}

		jtx, err := q.CreateJournalTransaction(r.Context(), db.CreateJournalTransactionParams{
			TransactionRef:    uuid.NewString(),
			TransferRequestID: trf.ID,
			IdempotencyKeyID:  idempotencyKey,
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
			IdempotencyKeyID:     idempotencyKey,
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
		newDestAvail := newDestLedger.Sub(newDestHeld)

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

		if err := api.saveIdem(r.Context(), params.IdempotencyKeyID, "withdraw_create", params.CustomerID, requestHash, payloadBytes, 202); err != nil {
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
		// Provider failed to even accept the request.
		// Manually trigger the failure logic to release the hold immediately.
		jsonData, _ := json.Marshal(PaystackTransferData{
			Reference:     trf.ID.String(),
			Status:        "failed",
			FailureReason: err.Error(),
		})
		api.handleTransferFailed(w, r, json.RawMessage(jsonData))
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

// func (api *ApiConfig) reverseWithdrawalHold(ctx context.Context, trfID pgtype.UUID, accountID pgtype.UUID, amount pgtype.Numeric, currency string) error {
// 	return api.Db.ExecTx(ctx, func(q *db.Queries) error {
// 		// 1. Lock balance projection for update
// 		balance, err := q.GetBalanceProjectionForUpdate(ctx, db.GetBalanceProjectionForUpdateParams{
// 			AccountID:    accountID,
// 			CurrencyCode: currency,
// 			BalanceKind:  "available",
// 		})
// 		if err != nil {
// 			return fmt.Errorf("lookup balance for reversal: %w", err)
// 		}

// 		// 2. Calculate reversed balances
// 		decLedger, _ := decimal.NewFromString(NumericToString(balance.LedgerBalance))
// 		decHeld, _ := decimal.NewFromString(NumericToString(balance.HeldBalance))
// 		decAmt, _ := decimal.NewFromString(NumericToString(amount))

// 		newHeld := decHeld.Sub(decAmt)
// 		newAvail := decLedger.Sub(newHeld) // Available = Ledger - Held

// 		newHeldNum, _ := StringToNumeric(newHeld.String())
// 		newAvailNum, _ := StringToNumeric(newAvail.String())

// 		// 3. Update projection and transfer status
// 		err = q.UpsertBalanceProjectionWithExpectedVersion(ctx, db.UpsertBalanceProjectionWithExpectedVersionParams{
// 			AccountID:        accountID,
// 			CurrencyCode:     currency,
// 			BalanceKind:      "available",
// 			LedgerBalance:    balance.LedgerBalance,
// 			AvailableBalance: newAvailNum,
// 			HeldBalance:      newHeldNum,
// 			ExpectedVersion:  balance.Version,
// 		})
// 		if err != nil {
// 			return fmt.Errorf("updating projection for reversal: %w", err)
// 		}

// 		_, err = q.UpdateTransferRequestStatus(ctx, db.UpdateTransferRequestStatusParams{
// 			Status: "reversed",
// 			ID:     trfID,
// 		})
// 		return err
// 	})
// }

func (api *ApiConfig) handleTransferFailed(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	var transferData PaystackTransferData
	json.Unmarshal(data, &transferData)

	trfID, _ := StringtoPgUuid(transferData.Reference)

	err := api.Db.ExecTx(r.Context(), func(q *db.Queries) error {

		trf, err := q.GetTransferRequestByIDForUpdate(r.Context(), trfID)
		if err != nil {
			return err
		}
		if trf.Status == "failed" {
			return nil
		}

		hold, err := q.GetActiveHoldByTransferRequestID(r.Context(), trf.ID)
		if err != nil {
			return err
		}

		balance, err := q.GetBalanceProjectionForUpdate(r.Context(),
			db.GetBalanceProjectionForUpdateParams{
				AccountID:    trf.SourceAccountID,
				CurrencyCode: trf.CurrencyCode,
				BalanceKind:  "available",
			})
		if err != nil {
			return err
		}

		// Release hold — funds return to available
		_, err = q.ReleaseHold(r.Context(), db.ReleaseHoldParams{
			ID:     hold.ID,
			Amount: trf.Amount,
		})
		if err != nil {
			return err
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

		err = q.UpsertBalanceProjectionWithExpectedVersion(r.Context(),
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
			return err
		}

		_, err = q.UpdateTransferRequestStatus(r.Context(), db.UpdateTransferRequestStatusParams{
			ID:     trf.ID,
			Status: "failed",
		})
		if err != nil {
			return err
		}

		payload, _ := json.Marshal(map[string]string{
			"transfer_id":    trf.ID.String(),
			"failure_reason": trf.FailureReason.String,
		})
		_, err = q.CreateOutboxEvent(r.Context(), db.CreateOutboxEventParams{
			AggregateType: "transfer_request",
			AggregateID:   trf.ID,
			EventType:     "transfer.failed",
			Payload:       payload,
			Headers:       []byte("{}"),
		})
		return err
	})

	if errors.Is(err, ErrAlreadyProcessed) {
		respondeWithJson(w, 200, map[string]string{"received": "true"})
		return
	}
	if err != nil {
		respondWithError(w, 500, err.Error())
		return
	}
	respondeWithJson(w, 200, map[string]string{"received": "true"})
}

func (api *ApiConfig) handleTransferSuccess(w http.ResponseWriter, r *http.Request, data json.RawMessage) {
	var transferData PaystackTransferData
	if err := json.Unmarshal(data, &transferData); err != nil {
		respondWithError(w, 400, "invalid transfer data")
		return
	}

	// Reference is the transfer_request_id you sent Paystack
	trfID, err := StringtoPgUuid(transferData.Reference)
	if err != nil {
		respondWithError(w, 400, "invalid reference")
		return
	}

	// zero, _ := StringToNumeric("0.00")

	err = api.Db.ExecTx(r.Context(), func(q *db.Queries) error {

		// 1. Lock the transfer request
		trf, err := q.GetTransferRequestByIDForUpdate(r.Context(), trfID)
		if err != nil {
			return fmt.Errorf("transfer not found: %w", err)
		}

		// Guard: already processed (webhook can arrive twice — Paystack retries)
		if trf.Status == "posted" {
			return fmt.Errorf("Transferr already processed")
		}
		Meta, err := json.Marshal(map[string]string{"transfer_code": transferData.TransferCode})
		if err != nil {
			return fmt.Errorf("Unable to parse Metadata payload: %w", err)
		}

		// 2. Get the active hold for this transfer
		hold, err := q.GetActiveHoldByTransferRequestID(r.Context(), trf.ID)
		if err != nil {
			return fmt.Errorf("hold not found: %w", err)
		}

		// 3. Lock balance projection
		balance, err := q.GetBalanceProjectionForUpdate(r.Context(),
			db.GetBalanceProjectionForUpdateParams{
				AccountID:    trf.SourceAccountID,
				CurrencyCode: trf.CurrencyCode,
				BalanceKind:  "available",
			})
		if err != nil {
			return err
		}

		// 4. Create the SETTLEMENT journal transaction — the real debit
		jtx, err := q.CreateJournalTransaction(r.Context(), db.CreateJournalTransactionParams{
			TransactionRef:    uuid.NewString(),
			TransferRequestID: trf.ID,
			IdempotencyKeyID:  trf.IdempotencyKeyID,
			Status:            "pending",
			EntryType:         "withdrawal",
			AccountingDate:    pgtype.Date{Time: time.Now(), Valid: true},
			EffectiveAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
			SourceSystem:      "paystack_webhook",
			SourceEventID:     pgtype.Text{String: transferData.TransferCode, Valid: true},
			Metadata:          Meta,
		})
		if err != nil {
			return err
		}

		settlementAcct, err := q.GetAccountByExternalRef(r.Context(),
			fmt.Sprintf("system_settlement_%s", trf.CurrencyCode))
		if err != nil {
			return err
		}

		// 5. Journal lines — actual debit of customer, credit settlement
		_, err = q.CreateJournalLine(r.Context(), db.CreateJournalLineParams{
			JournalTransactionID: jtx.ID,
			LineNumber:           1,
			AccountID:            trf.SourceAccountID,
			Side:                 "debit",
			Amount:               trf.Amount,
			CurrencyCode:         trf.CurrencyCode,
			BalanceKind:          "ledger",
			Memo:                 pgtype.Text{String: "withdrawal settled", Valid: true},
			Metadata:             nil,
		})
		if err != nil {
			return err
		}

		journalLine, err := q.CreateJournalLine(r.Context(), db.CreateJournalLineParams{
			JournalTransactionID: jtx.ID,
			LineNumber:           2,
			AccountID:            settlementAcct.ID,
			Side:                 "credit",
			Amount:               trf.Amount,
			CurrencyCode:         trf.CurrencyCode,
			BalanceKind:          "ledger",
			Memo:                 pgtype.Text{String: "withdrawal settled", Valid: true},
			Metadata:             nil,
		})
		if err != nil {
			return err
		}

		// 6. Consume the hold
		_, err = q.ConsumeHold(r.Context(), db.ConsumeHoldParams{
			ID:     hold.ID,
			Amount: trf.Amount,
		})
		if err != nil {
			return err
		}

		// 7. Mark journal transaction posted
		_, err = q.MarkJournalTransactionPosted(r.Context(), jtx.ID)
		if err != nil {
			return err
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

		err = q.UpsertBalanceProjectionWithExpectedVersion(r.Context(),
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
			return err
		}

		// 9. Update transfer request → posted
		_, err = q.UpdateTransferRequestStatus(r.Context(), db.UpdateTransferRequestStatusParams{
			ID:     trf.ID,
			Status: "posted",
		})
		if err != nil {
			return err
		}

		payload, err := json.Marshal(map[string]string{
			"transfer_id":        trf.ID.String(),
			"amount":             NumericToString(trf.Amount),
			"currency":           trf.CurrencyCode,
			"provider_reference": transferData.TransferCode,
		})
		if err != nil {
			return fmt.Errorf("Unable to parse Metadata payload: %w", err)
		}
		header, err := json.Marshal(map[string]string{})

		// 10. Outbox event — client gets notified via this
		_, err = q.CreateOutboxEvent(r.Context(), db.CreateOutboxEventParams{
			AggregateType: "transfer_request",
			AggregateID:   trf.ID,
			EventType:     "transfer.posted",
			Payload:       payload,
			Headers:       header,
		})
		return err
	})

	if errors.Is(err, ErrAlreadyProcessed) {
		respondeWithJson(w, 200, map[string]string{"received": "true"})
		return
	}
	if err != nil {
		// Return 500 — Paystack will retry the webhook later
		respondWithError(w, 500, err.Error())
		return
	}

	respondeWithJson(w, 200, map[string]string{"received": "true"})
}
