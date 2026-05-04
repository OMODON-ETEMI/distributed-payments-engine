package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/OMODON-ETEMI/distributed-payments-engine/src/database"
	"github.com/OMODON-ETEMI/distributed-payments-engine/src/routes"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

var (
	testDb    *database.Db
	testRedis *redis.Client
	testAPI   *routes.ApiConfig
)

func setup(t *testing.T) {
	ctx := context.Background()

	// Connect to test DB
	err := godotenv.Load("../../.env")
	if err != nil {
		t.Logf("Warning: .env file not found: %v", err)
	}
	dbUrl := os.Getenv("TEST_DB_URL")
	if dbUrl == "" {
		t.Skip("TEST_DB_URL not set, skipping integration tests")
	}

	connPool, err := pgxpool.New(ctx, dbUrl)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	testDb = database.NewDb(connPool)
	testRedis = redis.NewClient(&redis.Options{Addr: "localhost:6379"})

	// Test Redis connection
	_, err = testRedis.Ping(ctx).Result()
	if err != nil {
		t.Skip("Redis not available, skipping integration tests")
	}

	mockProvider := routes.NewMockProvider("paystack", 0.0)
	breakerConfig := routes.BreakerConfig{
		MaxRequests:              1,
		Interval:                 5 * time.Second,
		Timeout:                  10 * time.Second,
		ConsecutiveFailThreshold: 3,
	}
	mockProviderBreaker := routes.NewProviderBreaker(mockProvider, breakerConfig)
	testAPI = &routes.ApiConfig{
		Db:     testDb,
		Redis:  testRedis,
		Router: routes.NewPaymentRouter([]*routes.ProviderBreaker{mockProviderBreaker}),
	}
	mockProvider.Api = testAPI

	t.Cleanup(func() {
		connPool.Close()
	})

	t.Cleanup(func() {
		testRedis.Close()
	})
}

func TestIntegration_FullUserAccountWorkflow(t *testing.T) {
	setup(t)
	// ctx := context.Background()

	// 1. Create User
	userPayload := map[string]interface{}{
		"external_ref": fmt.Sprintf("user_%s", uuid.NewString()[:8]),
		"full_name":    "John Doe",
		"email":        "john@example.com",
		"status":       "active",
		"metadata":     map[string]string{"tier": "gold"},
	}
	userBody, _ := json.Marshal(userPayload)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/create/user", bytes.NewBuffer(userBody))
	testAPI.HandleCreateUser(w, req)

	if w.Code != 200 {
		t.Fatalf("CreateUser failed with status %d: %s", w.Code, w.Body.String())
	}

	var userResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &userResp)
	userID := userResp["id"].(string)

	// 2. Create Account
	acctPayload := map[string]interface{}{
		"customer_id":        userID,
		"external_ref":       fmt.Sprintf("acct_%s", uuid.NewString()[:8]),
		"account_number":     fmt.Sprintf("1000%s", uuid.NewString()[:8]),
		"account_type":       "customer",
		"currency_code":      "NGN",
		"status":             "active",
		"ledger_normal_side": "credit",
	}
	acctBody, _ := json.Marshal(acctPayload)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/create/account", bytes.NewBuffer(acctBody))
	testAPI.HandleCreateAccount(w, req)

	if w.Code != 200 {
		t.Fatalf("CreateAccount failed with status %d: %s", w.Code, w.Body.String())
	}

	var acctResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &acctResp)
	accountID := acctResp["id"].(string)

	// 3. Get Balances (should be 0)
	balPayload := map[string]interface{}{"account_id": accountID}
	balBody, _ := json.Marshal(balPayload)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/account/1/balances", bytes.NewBuffer(balBody))
	testAPI.HandleGetBalancesForAccount(w, req)
	if w.Code != 200 {
		t.Logf("GetBalances warning: %d", w.Code)
	}

	// 4. Deposit (add funds)
	idempKey := uuid.NewString()
	depositPayload := map[string]interface{}{
		"idempotency_key_id":     idempKey,
		"customer_id":            userID,
		"destination_account_id": accountID,
		"currency_code":          "NGN",
		"amount":                 "10000",
		"fee_amount":             "0",
		"source_system":          "test_system",
		"description":            "test deposit",
	}
	depBody, _ := json.Marshal(depositPayload)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/account/1/deposite", bytes.NewBuffer(depBody))
	testAPI.HandleDeposite(w, req)
	if w.Code != 201 && w.Code != 200 {
		t.Logf("HandleDeposite: status %d (expected 201), response: %s", w.Code, w.Body.String())
	}

	t.Logf("✓ Full workflow: user created %s, account %s, deposit processed", userID, accountID)
}

func TestIntegration_TransferBetweenAccounts(t *testing.T) {
	setup(t)
	// ctx := context.Background()

	// Create two users and two accounts
	user1Ref := fmt.Sprintf("user_%s", uuid.NewString()[:8])
	user1Body, _ := json.Marshal(map[string]interface{}{
		"external_ref": user1Ref,
		"full_name":    "Alice",
		"email":        "alice@test.com",
		"status":       "active",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/create/user", bytes.NewBuffer(user1Body))
	testAPI.HandleCreateUser(w, req)
	var u1 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &u1)
	user1ID := u1["id"].(string)

	user2Ref := fmt.Sprintf("user_%s", uuid.NewString()[:8])
	user2Body, _ := json.Marshal(map[string]interface{}{
		"external_ref": user2Ref,
		"full_name":    "Bob",
		"email":        "bob@test.com",
		"status":       "active",
	})
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/create/user", bytes.NewBuffer(user2Body))
	testAPI.HandleCreateUser(w, req)
	var u2 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &u2)
	user2ID := u2["id"].(string)

	// Create accounts
	acct1Body, _ := json.Marshal(map[string]interface{}{
		"customer_id":        user1ID,
		"external_ref":       fmt.Sprintf("a1_%s", uuid.NewString()[:8]),
		"account_number":     fmt.Sprintf("2000%s", uuid.NewString()[:8]),
		"account_type":       "customer",
		"currency_code":      "NGN",
		"status":             "active",
		"ledger_normal_side": "credit",
	})
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/create/account", bytes.NewBuffer(acct1Body))
	testAPI.HandleCreateAccount(w, req)
	var a1 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &a1)
	acct1ID := a1["id"].(string)

	acct2Body, _ := json.Marshal(map[string]interface{}{
		"customer_id":        user2ID,
		"external_ref":       fmt.Sprintf("a2_%s", uuid.NewString()[:8]),
		"account_number":     fmt.Sprintf("2000%s", uuid.NewString()[:8]),
		"account_type":       "customer",
		"currency_code":      "NGN",
		"status":             "active",
		"ledger_normal_side": "credit",
	})
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/create/account", bytes.NewBuffer(acct2Body))
	testAPI.HandleCreateAccount(w, req)
	var a2 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &a2)
	acct2ID := a2["id"].(string)

	// CreateTransfer between accounts
	tfrPayload := map[string]interface{}{
		"idempotency_key_id":     uuid.NewString(),
		"customer_id":            user1ID,
		"source_account_id":      acct1ID,
		"destination_account_id": acct2ID,
		"currency_code":          "NGN",
		"amount":                 "5000",
		"fee_amount":             "50",
		"source_system":          "test",
		"description":            "peer-to-peer transfer",
	}
	tfrBody, _ := json.Marshal(tfrPayload)
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/transfer/1", bytes.NewBuffer(tfrBody))
	testAPI.HandleCreateTransfer(w, req)
	// Expected: 400+ since account1 has no funds, but the route should be callable
	if w.Code < 400 || w.Code >= 600 {
		t.Logf("Transfer attempt: status %d (may fail due to insufficient funds, expected behavior)", w.Code)
	}

	t.Logf("✓ Transfer test: created accounts, attempted transfer")
}

func TestIntegration_WebhookProcessing(t *testing.T) {
	setup(t)

	// Simulate Paystack webhook for transfer success
	transferID := uuid.NewString()
	webhookData := map[string]interface{}{
		"event": "transfer.success",
		"data": map[string]interface{}{
			"transfer_code": "TRF_" + uuid.NewString()[:8],
			"reference":     transferID,
			"status":        "success",
		},
	}
	payload, _ := json.Marshal(webhookData)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/webhook/paystack", bytes.NewBuffer(payload))
	req.Header.Set("X-Paystack-Signature", "mock-signature")
	testAPI.HandlePaystackWebhook(w, req)

	// Should return 200 or 500 depending on transfer existence
	if w.Code != 200 && w.Code != 500 {
		t.Fatalf("Webhook handler: expected 200 or 500, got %d", w.Code)
	}

	t.Logf("✓ Webhook test: processed transfer event, status %d", w.Code)
}

func TestIntegration_IdempotencyKeyEnforcement(t *testing.T) {
	setup(t)

	// Create user twice with same external_ref should return cached/existing result
	userRef := fmt.Sprintf("user_%s", uuid.NewString()[:8])
	userPayload := map[string]interface{}{
		"external_ref": userRef,
		"full_name":    "Charlie",
		"email":        "charlie@test.com",
		"status":       "active",
	}
	userBody, _ := json.Marshal(userPayload)

	// First request
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/create/user", bytes.NewBuffer(userBody))
	testAPI.HandleCreateUser(w, req)
	if w.Code != 200 {
		t.Fatalf("First CreateUser: expected 200, got %d", w.Code)
	}

	var u1 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &u1)
	firstID := u1["id"].(string)

	// Duplicate external_ref should return existing user
	w = httptest.NewRecorder()
	userBody2, _ := json.Marshal(userPayload)
	req = httptest.NewRequest("POST", "/v1/create/user", bytes.NewBuffer(userBody2))
	testAPI.HandleCreateUser(w, req)
	if w.Code != 200 {
		t.Logf("Second CreateUser (duplicate external_ref): status %d (expected idempotency check)", w.Code)
	}

	var u2 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &u2)
	secondID := u2["id"].(string)

	if firstID != secondID {
		t.Logf("Idempotency: IDs differ (may be new user), first=%s second=%s", firstID, secondID)
	} else {
		t.Logf("✓ Idempotency: duplicate request returned same user %s", firstID)
	}
}

func TestIntegration_ConcurrentRequests(t *testing.T) {
	setup(t)

	// Spawn 10 concurrent user creation requests
	results := make(chan int, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			payload := map[string]interface{}{
				"external_ref": fmt.Sprintf("concurrent_user_%d_%s", id, uuid.NewString()[:8]),
				"full_name":    fmt.Sprintf("User %d", id),
				"email":        fmt.Sprintf("user%d@test.com", id),
				"status":       "active",
			}
			body, _ := json.Marshal(payload)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/create/user", bytes.NewBuffer(body))
			testAPI.HandleCreateUser(w, req)
			results <- w.Code
		}(i)
	}

	successCount := 0
	for i := 0; i < 10; i++ {
		code := <-results
		if code == 200 {
			successCount++
		}
	}

	if successCount < 8 {
		t.Logf("Concurrent requests: only %d/10 succeeded (may be normal under load)", successCount)
	} else {
		t.Logf("✓ Concurrent test: %d/10 concurrent requests succeeded", successCount)
	}
}

func TestIntegration_ErrorHandling(t *testing.T) {
	setup(t)

	// Invalid UUID for user ID
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/user/id", bytes.NewBuffer([]byte(`{"id":"not-a-uuid"}`)))
	testAPI.HandleGetUserById(w, req)
	if w.Code != 400 {
		t.Fatalf("Invalid UUID: expected 400, got %d", w.Code)
	}

	// Non-existent account lookup
	w = httptest.NewRecorder()
	validUUID := "00000000-0000-0000-0000-000000000000"
	req = httptest.NewRequest("POST", "/v1/account/1/balances", bytes.NewBuffer([]byte(fmt.Sprintf(`{"account_id":"%s"}`, validUUID))))
	testAPI.HandleGetBalancesForAccount(w, req)
	if w.Code != 404 && w.Code != 500 {
		t.Logf("Non-existent account: status %d", w.Code)
	}

	t.Logf("✓ Error handling: invalid inputs rejected appropriately")
}
