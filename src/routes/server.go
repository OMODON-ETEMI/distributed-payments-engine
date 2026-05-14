package routes

import (
	"github.com/OMODON-ETEMI/distributed-payments-engine/src/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type MessageProducer interface {
	SendMessage(topic, key string, payload []byte) error
	Close()
}

type ApiConfig struct {
	Kafka_producer MessageProducer
	Db             *database.Db
	DbPool         *pgxpool.Pool
	Redis          *redis.Client
	Router         *PaymentRouter
}

func NewRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})
}
