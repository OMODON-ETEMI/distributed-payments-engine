package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/OMODON-ETEMI/distributed-payments-engine/docs"
	db "github.com/OMODON-ETEMI/distributed-payments-engine/src/database/gen"
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

	dbUrl := os.Getenv("DB_URL")
	if dbUrl == "" {
		log.Fatal("DB_URL is not found in the enviroment")
	}

	ctx := context.Background()
	connPool, err := pgxpool.New(ctx, dbUrl)
	if err != nil {
		log.Fatal("Unable to connect to database:", err)
	}
	defer connPool.Close()

	api := &routes.ApiConfig{
		Db: db.New(connPool),
	}

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
	v1Router.Get("/healthz", routes.HandleReadiness)
	v1Router.Get("/err", routes.HandleError)
	v1Router.Post("/create/user", api.HandleCreateUser)
	v1Router.Post("/get/user/external_ref", api.HandleGetUserByExternalRef)
	v1Router.Post("/get/user/id", api.HandleGetUserById)
	v1Router.Post("/list/users", api.HandleListCustomers)
	v1Router.Post("/create/account", api.HandleCreateAccount)
	v1Router.Post("/get/account/external_ref", api.HandleGetAccountByExternalRef)
	v1Router.Post("/get/account/id", api.HandleGetAccountByID)
	v1Router.Post("/get/account/number", api.HandleGetAccountByAccountNumber)
	v1Router.Post("/list/accounts/customer", api.HandleListAccountByCustomer)
	v1Router.Post("/update/user", api.HandleUserUpdateStatus)
	v1Router.Post("/update/account", api.HandleUpdateAccountStatus)
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
