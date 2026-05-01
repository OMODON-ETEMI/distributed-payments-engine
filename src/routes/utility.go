package routes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

type IdemResult struct {
	ShouldProceed  bool
	CachedResponse []byte
	StatusCode     int
}

func (api *ApiConfig) IdemCheck(ctx context.Context, key string, ID string, requestHash string, scope string) (IdemResult, error) {
	redisKey := fmt.Sprintf("idem:%v:%v", ID, key)

	record, err := api.Db.Queries.GetIdempotencyKeyByScopeAndKey(ctx, db.GetIdempotencyKeyByScopeAndKeyParams{
		IdempotencyKey: key,
		Scope:          scope,
	})

	if err == nil {
		encodeHash := hex.EncodeToString(record.RequestHash)
		if encodeHash != requestHash {
			return IdemResult{}, fmt.Errorf("idempotency key reuse with different payload")
		}

		return IdemResult{
			ShouldProceed:  false,
			CachedResponse: record.ResponseBody,
			StatusCode:     int(record.ResponseCode.Int32),
		}, nil
	}

	ok, err := api.Redis.SetNX(ctx, redisKey, "processing", 1*time.Minute).Result() // Short TTL for lock
	if err != nil {
		return IdemResult{}, err
	}

	if !ok {
		return IdemResult{ShouldProceed: false}, fmt.Errorf("request currently processing")
	}

	return IdemResult{ShouldProceed: true}, nil
}

func (api *ApiConfig) saveIdem(ctx context.Context, key string, scope string, userID string, requestHash string, response []byte, statusCode int) error {
	_, err := api.Db.Queries.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
		IdempotencyKey: key,
		Scope:          scope,
		RequestHash:    []byte(requestHash),
		ResponseCode:   pgtype.Int4{Int32: int32(statusCode), Valid: true},
		ResponseBody:   response,
		LockedAt:       pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		ExpiresAt:      pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour).UTC(), Valid: true},
	})
	if err != nil {
		return err
	}
	redisKey := fmt.Sprintf("idem:%v:%v", userID, key)
	api.Redis.Del(ctx, redisKey)
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

func ValidateLedgerBalance(legs []JournalLeg) error {
	sum := decimal.Zero

	for _, leg := range legs {
		decAmount, err := NumericToDecimal(leg.Amount)
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		if leg.Side == "DEBIT" {
			sum = sum.Add(decAmount)
		} else if leg.Side == "CREDIT" {
			sum = sum.Sub(decAmount)
		}
	}

	if !sum.IsZero() {
		return fmt.Errorf("transaction unbalanced: remainder is %s", sum.String())
	}

	return nil
}

var ErrAlreadyProcessed = errors.New("already processed")
