package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/OMODON-ETEMI/distributed-payments-engine/docs"
	// db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
	"github.com/OMODON-ETEMI/distributed-payments-engine/src/database"
	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	godotenv.Load("../.env")

	portString := os.Getenv("PORT")
	if portString == "" {
		log.Fatal("PORT is not found in the enviroment")
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

	// Initialize Redis client
	redisClient := routes.NewRedis()

	// Configure and create the MockProvider
	mockProvider := routes.NewMockProvider("paystack", 0.3) // 30% failure rate for testing

	// Configure the Circuit Breaker for the MockProvider
	breakerConfig := routes.BreakerConfig{
		MaxRequests:              1,
		Interval:                 5 * time.Second,
		Timeout:                  10 * time.Second,
		ConsecutiveFailThreshold: 3, // Trip after 3 consecutive failures
	}
	mockProviderBreaker := routes.NewProviderBreaker(mockProvider, breakerConfig)

	api := &routes.ApiConfig{
		Db:     database.NewDb(connPool),
		Redis:  redisClient,
		Router: routes.NewPaymentRouter([]*routes.ProviderBreaker{mockProviderBreaker}),
	}

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

	v1Router := chi.NewRouter()
	// v1Router.Get("/healthz", routes.HandleReadiness)
	v1Router.Get("/err", routes.HandleError)
	v1Router.Post("/create/user", api.HandleCreateUser)
	v1Router.Get("/user/{id}", api.HandleGetUserById)
	v1Router.Post("/list/users", api.HandleListCustomers)
	v1Router.Post("/create/account", api.HandleCreateAccount)
	v1Router.Get("/account/number/{number}", api.HandleGetAccountByAccountNumber)
	v1Router.Post("/list/accounts/customer", api.HandleListAccountByCustomer)
	v1Router.Get("/account/{id}/balances", api.HandleGetBalancesForAccount)
	v1Router.Post("/account/deposite", api.HandleDeposite)
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
