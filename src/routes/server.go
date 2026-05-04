package routes

import (
	"github.com/OMODON-ETEMI/distributed-payments-engine/src/database"
	"github.com/redis/go-redis/v9"
)

type ApiConfig struct {
	Db     *database.Db
	Redis  *redis.Client
	Router *PaymentRouter
}

func NewRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})
}
