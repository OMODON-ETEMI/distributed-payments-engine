package routes

import (
	"time"

	database "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
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
