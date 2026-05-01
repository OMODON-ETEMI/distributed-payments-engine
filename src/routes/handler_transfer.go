package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

type TransferParams struct {
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
type JournalLeg struct {
	AccountID   pgtype.UUID
	Amount      pgtype.Numeric
	BalanceKind string
	Side        string
	Type        string
}

type GetTransferByIDParams struct {
	TransferID string `json:"transfer_id"`
}

func (api *ApiConfig) HandleCreateTransfer(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error reading request body: %v", err))
		return
	}
	requestHash := HashRequest(bodyBytes)

	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	decoder := json.NewDecoder(r.Body)
	params := TransferParams{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing Json: %v", err))
		return
	}
	if params.IdempotencyKeyID == "" || params.CustomerID == "" || params.SourceAccountID == "" || params.DestinationAccountID == "" || params.CurrencyCode == "" || params.Description == "" {
		respondWithError(w, 400, "missing required fields: idempotency_key_id, customer_id, source_account_id, destination_account_id, currency_code, description")
		return
	}
	idempotencyKey, err := StringtoPgUuid(params.IdempotencyKeyID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing idempkey to string: %v", err))
		return
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
	memo := pgtype.Text{String: params.Memo, Valid: params.Memo != ""}
	sourceAccountID, err := StringtoPgUuid(params.SourceAccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing source account ID: %v", err))
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
	destAccountID, err := StringtoPgUuid(params.DestinationAccountID)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing destination account ID: %v", err))
		return
	}
	destinationAccount, err := api.Db.Queries.GetAccountByID(r.Context(), destAccountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "destination account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up destination account: %v", err))
		return
	}
	systemAcct, err := api.Db.Queries.GetAccountByExternalRef(r.Context(), "system_fee_revenue_ngn")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondWithError(w, 404, "system fee account not found")
			return
		}
		respondWithError(w, 500, fmt.Sprintf("Error looking up system fee account: %v", err))
		return
	}
	if sourceAccount.CurrencyCode != params.CurrencyCode || destinationAccount.CurrencyCode != params.CurrencyCode {
		respondWithError(w, 400, "currency code does not match account currency")
		return
	}
	amount, err := StringToNumeric(params.Amount)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing amount: %v", err))
		return
	}
	feeAmount, err := StringToNumeric(params.FeeAmount)
	if err != nil {
		respondWithError(w, 400, fmt.Sprintf("Error parsing fee amount: %v", err))
		return
	}
	ClientReference := pgtype.Text{String: params.ClientReference, Valid: params.ClientReference != ""}
	ExternalReference := pgtype.Text{String: params.ExternalReference, Valid: params.ExternalReference != ""}
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
	var jtx db.JournalTransaction
	err = api.Db.ExecTx(r.Context(), func(q *db.Queries) error {
		check, err := api.IdemCheck(r.Context(), params.IdempotencyKeyID, params.CustomerID, requestHash, "transfer_create")
		if err != nil {
			return fmt.Errorf("Error checking idempotency key: %v", err)
		}
		if !check.ShouldProceed {
			return fmt.Errorf("idem response %v, status code %v ", check.CachedResponse, check.StatusCode)
		}
		sourceAcct, err := q.GetAccountByIDForUpdate(r.Context(), sourceAccountID)
		if err != nil {
			return fmt.Errorf("Error looking up source account: %v", err)
		}
		destinationAcct, err := q.GetAccountByIDForUpdate(r.Context(), destAccountID)
		if err != nil {
			return fmt.Errorf("Error looking up destination account: %v", err)
		}
		systemFeeAcct, err := q.GetAccountByIDForUpdate(r.Context(), systemAcct.ID)
		if err != nil {
			return fmt.Errorf("Error looking up system fee account: %v", err)
		}
		balance, err := q.GetBalanceProjectionForUpdate(r.Context(), db.GetBalanceProjectionForUpdateParams{
			AccountID:    sourceAcct.ID,
			CurrencyCode: sourceAcct.CurrencyCode,
			BalanceKind:  "available",
		})
		if err != nil {
			return fmt.Errorf("Error looking up source balance projection: %v", err)
		}
		avail, _ := decimal.NewFromString(NumericToString(balance.AvailableBalance))
		amt, _ := decimal.NewFromString(NumericToString(amount))
		fee, _ := decimal.NewFromString(NumericToString(feeAmount))
		needed := amt.Add(fee)
		if avail.Cmp(needed) < 0 {
			return fmt.Errorf("insufficient funds")
		}
		destBalance, err := q.GetBalanceProjectionForUpdate(r.Context(), db.GetBalanceProjectionForUpdateParams{
			AccountID:    destinationAcct.ID,
			CurrencyCode: params.CurrencyCode,
			BalanceKind:  "available",
		})
		if err != nil {
			return fmt.Errorf("Error looking up destination balance projection: %v", err)
		}
		destLedger, _ := decimal.NewFromString(NumericToString(destBalance.LedgerBalance))
		destHeld, _ := decimal.NewFromString(NumericToString(destBalance.HeldBalance))
		trf, err := q.CreateTransferRequest(r.Context(), db.CreateTransferRequestParams{
			IdempotencyKeyID:     idempotencyKey,
			CustomerID:           customer.ID,
			SourceAccountID:      sourceAcct.ID,
			DestinationAccountID: destinationAcct.ID,
			CurrencyCode:         params.CurrencyCode,
			Amount:               amount,
			FeeAmount:            feeAmount,
			ClientReference:      ClientReference,
			ExternalReference:    ExternalReference,
			Metadata:             metaBytes,
		})
		if err != nil {
			return fmt.Errorf("Error creating transfer request: %v", err)
		}

		jtx, err := q.CreateJournalTransaction(r.Context(), db.CreateJournalTransactionParams{
			TransactionRef:    uuid.NewString(),
			TransferRequestID: trf.ID,
			IdempotencyKeyID:  idempotencyKey,
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
			return fmt.Errorf("Error creating journal transaction: %v", err)
		}
		neededNumeric, err := StringToNumeric(needed.String())
		if err != nil {
			return fmt.Errorf("failed to parse amount: %w", err)
		}
		legs := []JournalLeg{
			{AccountID: sourceAcct.ID, Amount: neededNumeric, Side: "debit"},
			{AccountID: destinationAcct.ID, Amount: amount, Side: "credit"},
		}
		if fee.IsPositive() {
			legs = append(legs, JournalLeg{
				AccountID: systemFeeAcct.ID, // Supposed to be system fee account to be configured later
				Amount:    feeAmount,
				Side:      "credit",
			})
		}
		err = ValidateLedgerBalance(legs)
		if err != nil {
			return err
		}
		var sourceLineID, destLineID pgtype.UUID
		for index, leg := range legs {
			line, err := q.CreateJournalLine(r.Context(), db.CreateJournalLineParams{
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
			if leg.AccountID == sourceAcct.ID && leg.Side == "debit" {
				sourceLineID = line.ID
			}
			if leg.AccountID == destinationAcct.ID && leg.Side == "credit" {
				destLineID = line.ID
			}
		}

		currentLedger, err := decimal.NewFromString(NumericToString(balance.LedgerBalance))
		if err != nil {
			return fmt.Errorf("parsing ledger balance: %w", err)
		}
		currentHeld, err := decimal.NewFromString(NumericToString(balance.HeldBalance))
		if err != nil {
			return fmt.Errorf("parsing held balance: %w", err)
		}

		_, err = q.MarkJournalTransactionPosted(r.Context(), jtx.ID)
		if err != nil {
			return fmt.Errorf("Error Marking journal transaction: %v", err)
		}

		newSourceLedger := currentLedger.Sub(needed)
		newSourceHeld := currentHeld
		newSourceAvail := newSourceLedger.Sub(currentHeld)

		newSourceLedgerNum, err := StringToNumeric(newSourceLedger.String())
		newSourceAvailNum, err := StringToNumeric(newSourceAvail.String())
		newSourceHeldNum, err := StringToNumeric(newSourceHeld.String())

		newDestLedger := destLedger.Add(amt)
		newDestHeld := destHeld
		newDestAvail := newDestLedger.Add(destHeld)

		newDestLedgerNum, err := StringToNumeric(newDestLedger.String())
		newDestAvailNum, err := StringToNumeric(newDestAvail.String())
		newDestHeldNum, err := StringToNumeric(newDestHeld.String())

		err = q.UpsertBalanceProjectionWithExpectedVersion(r.Context(), db.UpsertBalanceProjectionWithExpectedVersionParams{
			AccountID:        sourceAcct.ID,
			CurrencyCode:     params.CurrencyCode,
			BalanceKind:      "available",
			LedgerBalance:    newSourceLedgerNum,
			AvailableBalance: newSourceAvailNum,
			HeldBalance:      newSourceHeldNum,
			LastTxID:         jtx.ID,
			LastLineID:       sourceLineID,
			ExpectedVersion:  balance.Version,
		})
		if err != nil {
			return fmt.Errorf("upserting source balance: %w", err)
		}
		err = q.UpsertBalanceProjectionWithExpectedVersion(r.Context(), db.UpsertBalanceProjectionWithExpectedVersionParams{
			AccountID:        destinationAcct.ID,
			CurrencyCode:     params.CurrencyCode,
			BalanceKind:      "available",
			LedgerBalance:    newDestLedgerNum,
			AvailableBalance: newDestAvailNum,
			HeldBalance:      newDestHeldNum,
			LastTxID:         jtx.ID,
			LastLineID:       destLineID,
			ExpectedVersion:  destBalance.Version,
		})
		if err != nil {
			return fmt.Errorf("upserting destination balance: %w", err)
		}

		trf, err = q.UpdateTransferRequestStatus(r.Context(), db.UpdateTransferRequestStatusParams{
			Status: "Posted",
			ID:     trf.ID,
		})
		if err != nil {
			return fmt.Errorf("Error Updating the transfer reuest status : %w", err)
		}

		// prepare outbox payload (JSON of the transfer) and headers
		payloadBytes, err := json.Marshal(trf)
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
		partitionKey := pgtype.Text{String: trf.ID.String(), Valid: true}

		_, err = q.CreateOutboxEvent(r.Context(), db.CreateOutboxEventParams{
			AggregateType:    "transfer_request",
			AggregateID:      trf.ID,
			EventType:        "transfer.posted",
			IdempotencyKeyID: idempotencyKey,
			Payload:          payloadBytes,
			Headers:          headersBytes,
			PartitionKey:     partitionKey,
		})
		if err != nil {
			return fmt.Errorf("failed to create outbox event: %w", err)
		}

		// save idempotency response (store JSON response and status code)
		if err := api.saveIdem(r.Context(), params.IdempotencyKeyID, "transfer_create", params.CustomerID, requestHash, payloadBytes, 201); err != nil {
			return fmt.Errorf("error saving idempotency key: %w", err)
		}
		return nil
	})
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("transfer failed: %v", err))
		return
	}
	respondeWithJson(w, 201, ToTransferResponse(trf, &jtx))
	return

}

func (api *ApiConfig) GetTransferbyID(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := GetTransferByIDParams{}
	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, 404, fmt.Sprintf("Error parsing json: %v", err))
		return
	}
	transferID, err := StringtoPgUuid(params.TransferID)
	if err != nil {
		respondWithError(w, 404, fmt.Sprintf("Error parsing ID to type PGUUID: %v", err))
		return
	}
	transfer, err := api.Db.Queries.GetTransferRequestByID(r.Context(), transferID)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error reading transfer from the database: %v", err))
		return
	}
	journalTransaction, err := api.Db.Queries.GetJournalTransactionByRef(r.Context(), params.TransferID)
	if err != nil {
		respondWithError(w, 500, fmt.Sprintf("Error looking up journal transaction: %v", err))
		return
	}
	respondeWithJson(w, 201, ToTransferResponse(transfer, &journalTransaction))
}
