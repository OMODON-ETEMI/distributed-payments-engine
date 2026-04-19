package routes

import (
	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/redis/go-redis/v9"
)

type ApiConfig struct {
	Db    *db.Queries
	Redis *redis.Client
}

func NewRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})
}
