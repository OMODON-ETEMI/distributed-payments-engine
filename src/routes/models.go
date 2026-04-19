package routes

import (
	"encoding/json"
	"time"

	database "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
)

type UserResponse struct {
	ID          string          `json:"id"`
	ExternalRef string          `json:"external_ref"`
	FullName    string          `json:"full_name"`
	Email       string          `json:"email"`
	Phone       string          `json:"phone"`
	NationalID  string          `json:"national_id"`
	Status      string          `json:"status"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	DeletedAt   time.Time       `json:"deleted_at"`
}

type AccountResponse struct {
	ID               string          `json:"id"`
	CustomerID       string          `json:"customer_id"`
	ExternalRef      string          `json:"external_ref"`
	AccountNumber    string          `json:"account_number"`
	AccountType      string          `json:"account_type"`
	Status           string          `json:"status"`
	Metadata         json.RawMessage `json:"metadata"`
	CurrencyCode     string          `json:"currency_code"`
	LedgerNormalSide string          `json:"ledger_normal_side"`
	OpenedAt         time.Time       `json:"opened_at"`
	ClosedAt         time.Time       `json:"closed_at"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	DeletedAt        time.Time       `json:"deleted_at"`
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
		Metadata:    dbUser.Metadata,
		CreatedAt:   dbUser.CreatedAt.Time,
		UpdatedAt:   dbUser.UpdatedAt.Time,
		DeletedAt:   dbUser.DeletedAt.Time,
	}
}

func AccountResponseObject(dbAccount database.Account) AccountResponse {
	return AccountResponse{
		ID:               dbAccount.ID.String(),
		CustomerID:       dbAccount.CustomerID.String(),
		ExternalRef:      dbAccount.ExternalRef,
		AccountNumber:    dbAccount.AccountNumber,
		AccountType:      dbAccount.AccountType,
		Status:           dbAccount.Status,
		CurrencyCode:     dbAccount.CurrencyCode,
		LedgerNormalSide: dbAccount.LedgerNormalSide,
		Metadata:         dbAccount.Metadata,
		CreatedAt:        dbAccount.CreatedAt.Time,
		OpenedAt:         dbAccount.OpenedAt.Time,
		UpdatedAt:        dbAccount.UpdatedAt.Time,
		ClosedAt:         dbAccount.ClosedAt.Time,
		DeletedAt:        dbAccount.DeletedAt.Time,
	}
}

type JournalTransactionResponse struct {
	ID                      string          `json:"id"`
	TransactionRef          string          `json:"transaction_ref"`
	TransferRequestID       string          `json:"transfer_request_id"`
	IdempotencyKeyID        string          `json:"idempotency_key_id"`
	Status                  string          `json:"status"`
	EntryType               string          `json:"entry_type"`
	AccountingDate          time.Time       `json:"accounting_date"`
	EffectiveAt             time.Time       `json:"effective_at"`
	PostedAt                time.Time       `json:"posted_at"`
	ReversedTransactionID   string          `json:"reversed_transaction_id"`
	ReversalOfTransactionID string          `json:"reversal_of_transaction_id"`
	SourceSystem            string          `json:"source_system"`
	SourceEventID           string          `json:"source_event_id"`
	Description             string          `json:"description"`
	Metadata                json.RawMessage `json:"metadata"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
	DeletedAt               time.Time       `json:"deleted_at"`
}

type JournalLineResponse struct {
	ID                   string          `json:"id"`
	JournalTransactionID string          `json:"journal_transaction_id"`
	LineNumber           int32           `json:"line_number"`
	AccountID            string          `json:"account_id"`
	Side                 string          `json:"side"`
	Amount               string          `json:"amount"`
	CurrencyCode         string          `json:"currency_code"`
	BalanceKind          string          `json:"balance_kind"`
	Memo                 string          `json:"memo"`
	Metadata             json.RawMessage `json:"metadata"`
	CreatedAt            time.Time       `json:"created_at"`
}

type BalanceProjectionResponse struct {
	AccountID        string    `json:"account_id"`
	CurrencyCode     string    `json:"currency_code"`
	BalanceKind      string    `json:"balance_kind"`
	LedgerBalance    string    `json:"ledger_balance"`
	AvailableBalance string    `json:"available_balance"`
	HeldBalance      string    `json:"held_balance"`
	Version          int64     `json:"version"`
	ComputedAt       time.Time `json:"computed_at"`
}

type ComputeLedgerBalanceResponse struct {
	LedgerBalance     string `json:"ledger_balance"`
	LastLineID        string `json:"last_line_id"`
	LastTransactionID string `json:"last_transaction_id"`
}

type HeldAmountResponse struct {
	HeldBalance string `json:"held_balance"`
}

func JournalTransactionResponseObject(j database.JournalTransaction) JournalTransactionResponse {
	return JournalTransactionResponse{
		ID:                      j.ID.String(),
		TransactionRef:          j.TransactionRef,
		TransferRequestID:       j.TransferRequestID.String(),
		IdempotencyKeyID:        j.IdempotencyKeyID.String(),
		Status:                  j.Status,
		EntryType:               j.EntryType,
		AccountingDate:          j.AccountingDate.Time,
		EffectiveAt:             j.EffectiveAt.Time,
		PostedAt:                j.PostedAt.Time,
		ReversedTransactionID:   j.ReversedTransactionID.String(),
		ReversalOfTransactionID: j.ReversalOfTransactionID.String(),
		SourceSystem:            j.SourceSystem,
		SourceEventID:           j.SourceEventID.String,
		Description:             j.Description.String,
		Metadata:                j.Metadata,
		CreatedAt:               j.CreatedAt.Time,
		UpdatedAt:               j.UpdatedAt.Time,
		DeletedAt:               j.DeletedAt.Time,
	}
}

func JournalLineResponseObject(l database.JournalLine) JournalLineResponse {
	amountStr := ""
	if s := formatNumeric(l.Amount); s != "" {
		amountStr = s
	}
	return JournalLineResponse{
		ID:                   l.ID.String(),
		JournalTransactionID: l.JournalTransactionID.String(),
		LineNumber:           l.LineNumber,
		AccountID:            l.AccountID.String(),
		Side:                 l.Side,
		Amount:               amountStr,
		CurrencyCode:         l.CurrencyCode,
		BalanceKind:          l.BalanceKind,
		Memo:                 l.Memo.String,
		Metadata:             l.Metadata,
		CreatedAt:            l.CreatedAt.Time,
	}
}

func BalanceProjectionResponseObject(b database.BalanceProjection) BalanceProjectionResponse {
	return BalanceProjectionResponse{
		AccountID:        b.AccountID.String(),
		CurrencyCode:     b.CurrencyCode,
		BalanceKind:      b.BalanceKind,
		LedgerBalance:    formatNumeric(b.LedgerBalance),
		AvailableBalance: formatNumeric(b.AvailableBalance),
		HeldBalance:      formatNumeric(b.HeldBalance),
		Version:          b.Version,
		ComputedAt:       b.ComputedAt.Time,
	}
}

func GetBalancesForAccountRowToResponse(r database.GetBalancesForAccountRow) BalanceProjectionResponse {
	return BalanceProjectionResponse{
		AccountID:        r.AccountID.String(),
		CurrencyCode:     r.CurrencyCode,
		BalanceKind:      r.BalanceKind,
		LedgerBalance:    formatNumeric(r.LedgerBalance),
		AvailableBalance: formatNumeric(r.AvailableBalance),
		HeldBalance:      formatNumeric(r.HeldBalance),
		Version:          r.Version,
		ComputedAt:       r.ComputedAt.Time,
	}
}

func ComputeLedgerBalanceRowToResponse(r database.ComputeLedgerBalanceRow) ComputeLedgerBalanceResponse {
	return ComputeLedgerBalanceResponse{
		LedgerBalance:     formatNumeric(r.LedgerBalance),
		LastLineID:        r.LastLineID.String(),
		LastTransactionID: r.LastTransactionID.String(),
	}
}
