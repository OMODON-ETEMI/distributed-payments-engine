package database

import (
	"context"
	"fmt"

	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Db struct {
	pool    *pgxpool.Pool
	Queries *db.Queries
}

func NewDb(pool *pgxpool.Pool) *Db {
	return &Db{
		pool:    pool,
		Queries: db.New(pool),
	}
}
func (db *Db) ExecTx(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return err
	}
	q := db.Queries.WithTx(tx)
	err = fn(q)
	if err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx error: %v, rb error: %v", err, rbErr)
		}
		return err
	}
	return tx.Commit(ctx)
}
