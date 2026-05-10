package routes

import (
	"github.com/OMODON-ETEMI/distributed-payments-engine/src/database"
	"github.com/OMODON-ETEMI/distributed-payments-engine/src/internal/messaging/producer"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type ApiConfig struct {
	Kafka  *producer.KafkaProducer
	Db     *database.Db
	DbPool *pgxpool.Pool
	Redis  *redis.Client
	Router *PaymentRouter
}

func NewRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})
}
