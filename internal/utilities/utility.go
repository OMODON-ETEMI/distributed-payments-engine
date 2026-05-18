package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database/gen"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

// JournalLeg represents a single entry in a balanced transaction
type JournalLeg struct {
	AccountID   pgtype.UUID
	Amount      pgtype.Numeric
	BalanceKind string
	Side        string
	Type        string
}

type IdemResult struct {
	ShouldProceed  bool
	IsProcessing   bool
	CachedResponse []byte
	StatusCode     int
}

type DepositParams struct {
	Provider             string                 `json:"provider"`
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

// IdemCheck verifies if a request has already been processed using Redis and Postgres.
// Returns ShouldProceed=true ONLY if this is the first time seeing this key.
// Handles three cases:
// 1. First request: Sets Redis lock, returns ShouldProceed=true
// 2. Concurrent duplicate: Returns cached response if available, else error
// 3. Retry after completion: Returns cached response
func IdemCheck(ctx context.Context, queries *db.Queries, rdb *redis.Client, key string, ID string, requestHash string, scope string) (IdemResult, error) {
	redisKey := fmt.Sprintf("idem:%v:%v", ID, key)

	// Try to acquire lock
	ok, err := rdb.SetNX(ctx, redisKey, "processing", 5*time.Minute).Result()
	if err != nil {
		return IdemResult{}, err
	}

	// Case 1: Successfully acquired lock - this is a new request
	if ok {
		return IdemResult{ShouldProceed: true}, nil
	}

	// Case 2: Lock already exists - either being processed or already completed
	// Query DB to check status
	record, err := queries.GetIdempotencyKeyByScopeAndKey(ctx, db.GetIdempotencyKeyByScopeAndKeyParams{
		IdempotencyKey: key,
		Scope:          scope,
	})

	if err != nil {
		// No DB record yet - must be processing but DB insert not yet complete
		// Return error to force retry with exponential backoff
		return IdemResult{ShouldProceed: false, IsProcessing: true}, fmt.Errorf("request is being processed, please retry")
	}

	// DB record exists - verify request hash matches
	dbHashString := string(record.RequestHash)
	if dbHashString != requestHash {
		return IdemResult{}, fmt.Errorf("idempotency key reuse with different payload")
	}

	// Record exists - must be completed (ResponseBody would have been set by SaveIdem)
	// Return cached response for retry
	return IdemResult{
		ShouldProceed:  false,
		CachedResponse: record.ResponseBody,
		StatusCode:     int(record.ResponseCode.Int32),
	}, nil
}

// SaveIdem updates the persistent idempotency record and cleans up the Redis lock.
func SaveIdem(ctx context.Context, queries *db.Queries, rdb *redis.Client, ID string, key string, id pgtype.UUID, response []byte, statusCode int) error {
	_, err := queries.UpdateIdempotencyKeyResponse(ctx, db.UpdateIdempotencyKeyResponseParams{
		ID:           id,
		ResponseCode: pgtype.Int4{Int32: int32(statusCode), Valid: true},
		ResponseBody: response,
	})
	if err != nil {
		return err
	}
	redisKey := fmt.Sprintf("idem:%v:%v", ID, key)
	err = rdb.Set(ctx, redisKey, "completed", 24*time.Hour).Err()
	if err != nil {
		return err
	}
	return nil
}

func StringtoPgUuid(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{Valid: false}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func StringToNumeric(s string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	f, ok := new(big.Float).SetString(s)
	if !ok {
		return n, fmt.Errorf("invalid numeric string")
	}
	err := n.Scan(f.Text('f', -1))
	return n, err
}

func HashRequest(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

func NumericToString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0.00"
	}
	v, err := n.Value()
	if err != nil {
		return "0.00"
	}
	return fmt.Sprintf("%v", v)
}

func NumericToDecimal(n pgtype.Numeric) (decimal.Decimal, error) {
	if !n.Valid {
		return decimal.Zero, nil
	}
	v, err := n.Value()
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromString(fmt.Sprintf("%v", v))
}

// ValidateLedgerBalance ensures the sum of debits and credits equals zero.
func ValidateLedgerBalance(legs []JournalLeg) error {
	sum := decimal.Zero

	for _, leg := range legs {
		decAmount, err := NumericToDecimal(leg.Amount)
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		switch leg.Side {
		case "debit":
			sum = sum.Add(decAmount)
		case "credit":
			sum = sum.Sub(decAmount)
		default:
			return fmt.Errorf("invalid side: %s", leg.Side)
		}
	}

	if !sum.IsZero() {
		return fmt.Errorf("transaction unbalanced: remainder is %s", sum.String())
	}

	return nil
}

// ProcessWithdrawalMessage logs an incoming withdrawal event from Kafka into the database.
func ProcessWithdrawalMessage(ctx context.Context, queries *db.Queries, msg *kafka.Message) error {
	var data struct {
		ID       string `json:"id"`
		Provider string `json:"provider"`
	}

	if err := json.Unmarshal(msg.Value, &data); err != nil {
		return err
	}
	// Generate mock headers to satisfy the NOT NULL constraint and consistency
	headerPayload, err := json.Marshal(map[string]interface{}{
		"X-Paystack-Signature":  fmt.Sprintf("Mock-Sg-%s", uuid.NewString()[:8]),
		"X-Paystack-Request-Id": data.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	_, err = queries.CreateIncomingWebhook(ctx, db.CreateIncomingWebhookParams{
		Provider:        data.Provider,
		EventType:       pgtype.Text{String: *msg.TopicPartition.Topic, Valid: true},
		ExternalEventID: pgtype.Text{String: data.ID, Valid: true},
		Payload:         msg.Value,
		Headers:         headerPayload,
	})
	if err != nil {
		return err
	}
	return nil
}

// ProcessDepositMessage logs an incoming deposit event from Kafka into the database.
func ProcessDepositMessage(ctx context.Context, queries *db.Queries, msg *kafka.Message) error {
	var data struct {
		Provider          string `json:"provider"`
		ExternalReference string `json:"external_reference"`
	}

	if err := json.Unmarshal(msg.Value, &data); err != nil {
		return err
	}
	headerPayload, err := json.Marshal(map[string]interface{}{
		"X-Paystack-Signature":  fmt.Sprintf("Mock-Sg-%s", uuid.NewString()[:8]),
		"X-Paystack-Request-Id": data.ExternalReference,
	})
	if err != nil {
		log.Printf("Failed to marshal headers for deposit message: %v", err)
		return err
	}

	// 1. Persist to DB (Idempotency check happens inside your queries)
	_, err = queries.CreateIncomingWebhook(ctx, db.CreateIncomingWebhookParams{
		Provider:        data.Provider,
		EventType:       pgtype.Text{String: *msg.TopicPartition.Topic, Valid: true},
		ExternalEventID: pgtype.Text{String: data.ExternalReference, Valid: true},
		Payload:         msg.Value,
		Headers:         headerPayload,
	})
	if err != nil {
		return err
	}
	return nil
}

var ErrAlreadyProcessed = errors.New("already processed")
