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
	"github.com/jackc/pgx/v5"
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
		dbHashString := string(record.RequestHash)
		if dbHashString != requestHash {
			return IdemResult{}, fmt.Errorf("idempotency key reuse with different payload")
		}
		return IdemResult{
			ShouldProceed:  false,
			CachedResponse: record.ResponseBody,
			StatusCode:     int(record.ResponseCode.Int32),
		}, nil
	}

	ok, err := api.Redis.SetNX(ctx, redisKey, "processing", 1*time.Minute).Result()
	if err != nil {
		return IdemResult{}, err
	}

	if !ok {
		return IdemResult{ShouldProceed: false}, fmt.Errorf("request currently processing")
	}

	return IdemResult{ShouldProceed: true}, nil
}

func (api *ApiConfig) saveIdem(ctx context.Context, ID string, key string, id pgtype.UUID, response []byte, statusCode int) error {
	_, err := api.Db.Queries.UpdateIdempotencyKeyResponse(ctx, db.UpdateIdempotencyKeyResponseParams{
		ID:           id,
		ResponseCode: pgtype.Int4{Int32: int32(statusCode), Valid: true},
		ResponseBody: response,
	})
	if err != nil {
		return err
	}
	redisKey := fmt.Sprintf("idem:%v:%v", ID, key)
	api.Redis.Del(ctx, redisKey)
	return nil
}

func GetOrCreateBalanceProjection(ctx context.Context, q *db.Queries, accountID pgtype.UUID, currency, kind string) (db.BalanceProjection, error) {
	bal, err := q.GetBalanceProjectionForUpdate(ctx, db.GetBalanceProjectionForUpdateParams{
		AccountID:    accountID,
		CurrencyCode: currency,
		BalanceKind:  kind,
	})
	if err == nil {
		return bal, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.BalanceProjection{}, err
	}
	zero, _ := StringToNumeric("0.00")
	err = q.UpsertBalanceProjectionWithExpectedVersion(ctx, db.UpsertBalanceProjectionWithExpectedVersionParams{
		AccountID:        accountID,
		CurrencyCode:     currency,
		BalanceKind:      kind,
		LedgerBalance:    zero,
		AvailableBalance: zero,
		HeldBalance:      zero,
		LastTxID:         pgtype.UUID{Valid: false},
		LastLineID:       pgtype.UUID{Valid: false},
		ExpectedVersion:  0,
	})
	if err != nil {
		return db.BalanceProjection{}, err
	}
	return q.GetBalanceProjectionForUpdate(ctx, db.GetBalanceProjectionForUpdateParams{
		AccountID: accountID, CurrencyCode: currency, BalanceKind: kind,
	})
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

var ErrAlreadyProcessed = errors.New("already processed")
