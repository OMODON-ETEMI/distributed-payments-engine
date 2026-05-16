package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/database"
	"github.com/OMODON-ETEMI/distributed-payments-engine/cmd/routes"
	_ "github.com/OMODON-ETEMI/distributed-payments-engine/docs"
	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/messaging/consumer"
	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/messaging/producer"
	internal "github.com/OMODON-ETEMI/distributed-payments-engine/internal/utilities"
	"github.com/OMODON-ETEMI/distributed-payments-engine/internal/worker"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// @title Distributed Payments Engine API
// @version 1.0.0
// @description A comprehensive distributed payment processing system with support for transfers, deposits, withdrawals, and holds.
// @host localhost:8080
// @BasePath /v1
// @query.collection.format multi

func main() {
	godotenv.Load("../.env")

	portString := os.Getenv("PORT")
	if portString == "" {
		log.Fatal("PORT is not found in the enviroment")
	}

	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		log.Fatal("KAFKA_BROKER is not found in the environment")
	}
	kafkaGroupID := os.Getenv("KAFKA_GROUP_ID")
	if kafkaGroupID == "" {
		kafkaGroupID = "payment-engine-workers"
	}

	dbUrl := os.Getenv("e2e_TEST_DB_URL")
	if dbUrl == "" {
		log.Fatal("DB_URL is not found in the enviroment")
	}

	ctx := context.Background()
	connPool, err := pgxpool.New(ctx, dbUrl)
	if err != nil {
		log.Fatal("Unable to connect to database:", err)
	}
	defer connPool.Close()

	// Initialize Kafka Producer
	kafkaProducer, err := producer.NewKafkaProducer(kafkaBroker)
	if err != nil {
		log.Fatalf("Failed to create Kafka producer: %v", err)
	}
	defer kafkaProducer.Close()

	// Initialize dedicated Kafka Consumers for each worker to avoid rebalance fights
	depositConsumer, err := consumer.NewKafkaConsumer(kafkaBroker, kafkaGroupID)
	if err != nil {
		log.Fatalf("Failed to create deposit consumer: %v", err)
	}
	defer depositConsumer.Consumer.Close()

	withdrawalConsumer, err := consumer.NewKafkaConsumer(kafkaBroker, kafkaGroupID)
	if err != nil {
		log.Fatalf("Failed to create withdrawal consumer: %v", err)
	}
	defer withdrawalConsumer.Consumer.Close()

	adminClient, err := kafka.NewAdminClient(&kafka.ConfigMap{"bootstrap.servers": kafkaBroker})
	if err != nil {
		log.Fatalf("Failed to create Kafka admin client: %v", err)
	}
	defer adminClient.Close()

	topics := []kafka.TopicSpecification{
		{Topic: "withdrawal.webhook", NumPartitions: 3, ReplicationFactor: 1},
		{Topic: "deposite.transfer", NumPartitions: 3, ReplicationFactor: 1},
	}

	_, err = adminClient.CreateTopics(ctx, topics)
	if err != nil {
		log.Printf("Topic creation note: %v", err)
	}
	// Initialize Redis client
	redisClient := routes.NewRedis()

	// Configure and create the MockProvider
	mockProvider := routes.NewMockProvider("paystack", 0.0) // 20% failure rate for testing

	// Configure the Circuit Breaker for the MockProvider
	breakerConfig := internal.BreakerConfig{
		MaxRequests:              1,
		Interval:                 5 * time.Second,
		Timeout:                  10 * time.Second,
		ConsecutiveFailThreshold: 3,
	}
	mockProviderBreaker := routes.NewProviderBreaker(mockProvider, breakerConfig)

	api := &routes.ApiConfig{
		Kafka_producer: kafkaProducer,
		Db:             database.NewDb(connPool),
		DbPool:         connPool,
		Redis:          redisClient,
		Router:         routes.NewPaymentRouter([]*routes.ProviderBreaker{mockProviderBreaker}),
	}

	// Start Kafka Background Worker
	go worker.StartWithdrawalKafkWorker(ctx, api, withdrawalConsumer)
	go worker.StartDepositKafkaWorker(ctx, api, depositConsumer)
	go worker.StartWebhookWorker(ctx, api)

	// Link the provider back to the API config for webhook simulation
	mockProvider.Api = api

	router := chi.NewRouter()

	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Serve Swagger UI
	// By importing the docs package, httpSwagger will automatically find the generated doc.json
	router.Get("/swagger/*", httpSwagger.Handler())

	v1Router := chi.NewRouter()
	v1Router.Get("/healthz", routes.HandleHealthCheck)
	v1Router.Post("/create/user", api.HandleCreateUser)
	v1Router.Get("/user/{id}", api.HandleGetUserById)
	v1Router.Post("/list/users", api.HandleListCustomers)
	v1Router.Post("/create/account", api.HandleCreateAccount)
	v1Router.Get("/account/number/{number}", api.HandleGetAccountByAccountNumber)
	v1Router.Post("/list/accounts/customer", api.HandleListAccountByCustomer)
	v1Router.Get("/account/{id}/balances", api.HandleGetBalancesForAccount)
	v1Router.Post("/account/deposit", api.HandleDeposit)
	v1Router.Post("/account/withdraw", api.HandleWithdraw)
	v1Router.Post("/account/transfer", api.HandleCreateTransfer)
	v1Router.Get("/transfer/{id}", api.GetTransferbyID)
	v1Router.Post("/update/user", api.HandleUserUpdateStatus)
	v1Router.Post("/update/account", api.HandleUpdateAccountStatus)
	v1Router.Post("/webhook/paystack", api.HandlePaystackWebhook)
	// Admin Endpoints
	v1Router.Get("/admin/hold/{transfer_id}", api.GetHoldBytransferRequest)
	v1Router.Post("/admin/hold/consume", api.ConsumeHold)
	router.Mount("/v1", v1Router)

	srv := &http.Server{
		Handler: router,
		Addr:    ":" + portString,
	}

	log.Printf("Server starting on port %v", portString)
	err = srv.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("PORT: %v", portString)
}
