package internal

import (
	"encoding/json"
	"time"

	database "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
)

// ── Shared primitives ────────────────────────────────────────────────

type MoneyAmount struct {
	Amount   string `json:"amount"`   // always a string — never float for money
	Currency string `json:"currency"` // "NGN"
}

type UserResponse struct {
	ID          string    `json:"id"`
	ExternalRef string    `json:"external_ref"`
	FullName    string    `json:"full_name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	NationalID  string    `json:"national_id"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type AccountParameters struct {
	ID               string                 `json:"id"`
	CustomerID       string                 `json:"customer_id"`
	ExternalRef      string                 `json:"external_ref"`
	AccountNumber    string                 `json:"account_number"`
	AccountType      string                 `json:"account_type"`
	Status           string                 `json:"status"`
	Metadata         map[string]interface{} `json:"metadata"`
	CurrencyCode     string                 `json:"currency_code"`
	LedgerNormalSide string                 `json:"ledger_normal_side"`
	Limit            int                    `json:"limit"`
	Offset           int                    `json:"offset"`
}

type UserParameters struct {
	IdempKey    string                 `json:"idemp_key"`
	ID          string                 `json:"id"`
	ExternalRef string                 `json:"external_ref"`
	FullName    string                 `json:"full_name"`
	Email       string                 `json:"email"`
	Phone       string                 `json:"phone"`
	NationalID  string                 `json:"national_id"`
	Status      string                 `json:"status"`
	Metadata    map[string]interface{} `json:"metadata"`
	Limit       int                    `json:"limit"`
	Offset      int                    `json:"offset"`
}

type AccountResponse struct {
	ID            string    `json:"id"`
	CustomerID    string    `json:"customer_id"`
	AccountNumber string    `json:"account_number"`
	AccountType   string    `json:"account_type"`
	Status        string    `json:"status"`
	Currency      string    `json:"currency_code"`
	NormalSide    string    `json:"ledger_normal_side"`
	CreatedAt     time.Time `json:"created_at"`
}

type BalanceResponse struct {
	AccountID string      `json:"account_id"`
	Currency  string      `json:"currency"`
	Ledger    MoneyAmount `json:"ledger_balance"`    // total ever posted
	Available MoneyAmount `json:"available_balance"` // spendable right now
	Held      MoneyAmount `json:"held_balance"`      // reserved, not yet debited
	AsOf      time.Time   `json:"as_of"`             // when projection was computed
}

type TransferResponse struct {
	ID                   string      `json:"id"`
	Status               string      `json:"status"`
	Amount               MoneyAmount `json:"amount"`
	Fee                  MoneyAmount `json:"fee"`
	SourceAccountID      string      `json:"source_account_id"`
	DestinationAccountID string      `json:"destination_account_id"`
	ClientReference      *string     `json:"client_reference,omitempty"`
	ExternalReference    *string     `json:"external_reference,omitempty"`
	Description          *string     `json:"description,omitempty"`
	JournalTransactionID *string     `json:"journal_transaction_id,omitempty"`
	RequestedAt          time.Time   `json:"requested_at"`
	PostedAt             *time.Time  `json:"posted_at,omitempty"`
	FailureCode          *string     `json:"failure_code,omitempty"`
	FailureReason        *string     `json:"failure_reason,omitempty"`
}

type WithdrawParams struct {
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

type HoldParams struct {
	ID                string  `json:"id"`
	TransferRequestID string  `json:"transfer_id"`
	Amount            string  `json:"amount"` // original hold amount
	ReasonCode        *string `json:"reason_code,omitempty"`
}

type WithdrawalResponse struct {
	Transfer         TransferResponse  `json:"transfer"`
	ProviderResponse *InitiateResponse `json:"provider_response"`
	Message          string            `json:"message"`
}

type FundingResponse struct {
	JournalTransactionID string      `json:"journal_transaction_id"`
	EntryType            string      `json:"entry_type"` // "deposit" or "withdrawal"
	Status               string      `json:"status"`     // "posted"
	Amount               MoneyAmount `json:"amount"`
	AccountID            string      `json:"account_id"`
	ExternalReference    *string     `json:"external_reference,omitempty"`
	PostedAt             time.Time   `json:"posted_at"`
}

type HoldResponse struct {
	ID         string      `json:"id"`
	AccountID  string      `json:"account_id"`
	Status     string      `json:"status"`           // "active" | "consumed" | "released" | "expired"
	Amount     MoneyAmount `json:"amount"`           // original hold amount
	Remaining  MoneyAmount `json:"remaining_amount"` // still locked
	Captured   MoneyAmount `json:"captured_amount"`  // converted to real debit
	Released   MoneyAmount `json:"released_amount"`  // returned to available
	ReasonCode *string     `json:"reason_code,omitempty"`
	ExpiresAt  *time.Time  `json:"expires_at,omitempty"`
	CapturedAt *time.Time  `json:"captured_at,omitempty"`
	ReleasedAt *time.Time  `json:"released_at,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

type OutboxEventResponse struct {
	ID               string                 `json:"id"`
	AggregateType    string                 `json:"aggregate_type"`
	AggregateID      string                 `json:"aggregate_id"`
	EventType        string                 `json:"event_type"`
	Payload          map[string]interface{} `json:"payload"`
	Headers          []byte                 `json:"headers"`
	IdempotencyKeyID string                 `json:"idempotency_key_id"`
	PartitionKey     string                 `json:"partition_key"`
}

type WebhookBody struct {
	Provider string          `json:"provider"`
	Event    string          `json:"event"`
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Data     json.RawMessage `json:"data" swaggertype:"object"`
}

type WebhookTransferData struct {
	ID            string          `json:"id"`
	Provider      string          `json:"provider"`
	Amount        string          `json:"Amount"`
	Currency      string          `json:"Currency"`
	Domain        string          `json:"Domain"`
	AccountNumber string          `json:"account_number"`
	BankCode      string          `json:"bank_code"`
	FullName      string          `json:"full_name"`
	Customer      json.RawMessage `json:"customer" swaggertype:"object"`
	Reference     string          `json:"reference"`
	Status        string          `json:"status"`
	FailureReason string          `json:"failure_reason"`
}

type Customer struct {
	ID            string `json:"id"`
	AccountNumber string `json:"account_number"`
	Email         string `json:"email"`
	FullName      string `json:"full_name"`
}

type BreakerConfig struct {
	MaxRequests              uint32        // Max requests allowed when half-open
	Interval                 time.Duration // Time window for counting failures
	Timeout                  time.Duration // How long to stay "Open" before trying again
	ConsecutiveFailThreshold uint32        // Number of failures to trip the breaker
}

type InitiateRequest struct {
	Amount        string // in the smallest unit e.g. "500000" kobo
	Currency      string
	RecipientCode string // bank account identifier
	Reference     string // YOUR reference — use transfer_request_id
	Reason        string
}

type InitiateResponse struct {
	ProviderReference string
	Status            string // "pending" — never "success" at this stage
	QueuedAt          time.Time
}

func UserResponseObject(dbUser database.Customer) UserResponse {
	return UserResponse{
		ID:          dbUser.ID.String(),
		ExternalRef: dbUser.ExternalRef,
		FullName:    dbUser.FullName,
		Email:       dbUser.Email.String,
		Phone:       dbUser.Phone.String,
		NationalID:  dbUser.NationalID.String,
		Status:      dbUser.Status,
		CreatedAt:   dbUser.CreatedAt.Time,
	}
}

func AccountResponseObject(dbAccount database.Account) AccountResponse {
	return AccountResponse{
		ID:            dbAccount.ID.String(),
		CustomerID:    dbAccount.CustomerID.String(),
		AccountNumber: dbAccount.AccountNumber,
		AccountType:   dbAccount.AccountType,
		Status:        dbAccount.Status,
		CreatedAt:     dbAccount.CreatedAt.Time,
		Currency:      dbAccount.CurrencyCode,
	}
}

func ToBalanceResponse(accountID, currency string, row database.GetBalancesForAccountRow) BalanceResponse {
	return BalanceResponse{
		AccountID: accountID,
		Currency:  currency,
		Ledger:    MoneyAmount{Amount: NumericToString(row.LedgerBalance), Currency: currency},
		Available: MoneyAmount{Amount: NumericToString(row.AvailableBalance), Currency: currency},
		Held:      MoneyAmount{Amount: NumericToString(row.HeldBalance), Currency: currency},
		AsOf:      row.ComputedAt.Time,
	}
}

func ToTransferResponse(t database.TransferRequest, jtxID *database.JournalTransaction) TransferResponse {
	r := TransferResponse{
		ID:                   t.ID.String(),
		Status:               t.Status,
		Amount:               MoneyAmount{Amount: NumericToString(t.Amount), Currency: t.CurrencyCode},
		Fee:                  MoneyAmount{Amount: NumericToString(t.FeeAmount), Currency: t.CurrencyCode},
		SourceAccountID:      t.SourceAccountID.String(),
		DestinationAccountID: t.DestinationAccountID.String(),
		RequestedAt:          t.RequestedAt.Time,
	}
	if t.ClientReference.Valid {
		r.ClientReference = &t.ClientReference.String
	}
	if t.ExternalReference.Valid {
		r.ExternalReference = &t.ExternalReference.String
	}
	if t.FailureCode.Valid {
		r.FailureCode = &t.FailureCode.String
	}
	if t.FailureReason.Valid {
		r.FailureReason = &t.FailureReason.String
	}
	if t.PostedAt.Valid {
		r.PostedAt = &t.PostedAt.Time
	}
	if jtxID != nil {
		s := jtxID.ID.String()
		r.JournalTransactionID = &s
	}
	return r
}

func ToFundingResponse(jtx database.JournalTransaction, amount, currency, accountID string) FundingResponse {
	r := FundingResponse{
		JournalTransactionID: jtx.ID.String(),
		EntryType:            jtx.EntryType,
		Status:               jtx.Status,
		Amount:               MoneyAmount{Amount: amount, Currency: currency},
		AccountID:            accountID,
		PostedAt:             jtx.PostedAt.Time,
	}
	if jtx.SourceEventID.Valid {
		r.ExternalReference = &jtx.SourceEventID.String
	}
	return r
}

func ToHoldResponse(h database.FundsHold) HoldResponse {
	r := HoldResponse{
		ID:        h.ID.String(),
		AccountID: h.AccountID.String(),
		Status:    h.Status,
		Amount:    MoneyAmount{Amount: NumericToString(h.Amount), Currency: h.CurrencyCode},
		Remaining: MoneyAmount{Amount: NumericToString(h.RemainingAmount), Currency: h.CurrencyCode},
		Captured:  MoneyAmount{Amount: NumericToString(h.CapturedAmount), Currency: h.CurrencyCode},
		Released:  MoneyAmount{Amount: NumericToString(h.ReleasedAmount), Currency: h.CurrencyCode},
		CreatedAt: h.CreatedAt.Time,
	}
	if h.ReasonCode.Valid {
		r.ReasonCode = &h.ReasonCode.String
	}
	if h.ExpiresAt.Valid {
		r.ExpiresAt = &h.ExpiresAt.Time
	}
	if h.CapturedAt.Valid {
		r.CapturedAt = &h.CapturedAt.Time
	}
	if h.ReleasedAt.Valid {
		r.ReleasedAt = &h.ReleasedAt.Time
	}
	return r
}
