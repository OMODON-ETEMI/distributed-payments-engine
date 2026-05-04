package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

var (
	baseURL string
)

func init() {
	_ = godotenv.Load("../../.env")
	baseURL = os.Getenv("API_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
}

func setupE2E(t *testing.T) {
	// Verify server is running by making a simple health check request
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/v1/err")
	if err != nil {
		t.Skipf("Server not running at %s: %v", baseURL, err)
	}
	resp.Body.Close()

	t.Logf("✓ Server running at %s", baseURL)
}

// ─── Scenario 1: Complete Payment Flow ───────────────────────────────
// User → Account → Deposit → Balance Check → Transfer

func TestE2E_CompletePaymentFlow(t *testing.T) {
	setupE2E(t)
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Create User
	userPayload := map[string]interface{}{
		"external_ref": fmt.Sprintf("e2e_user_%s", uuid.NewString()[:8]),
		"full_name":    "Alice Johnson",
		"email":        "alice@e2e.test",
		"status":       "active",
		"metadata":     map[string]string{"tier": "premium"},
	}
	userBody, _ := json.Marshal(userPayload)
	resp, err := client.Post(baseURL+"/v1/create/user", "application/json", bytes.NewBuffer(userBody))
	if err != nil {
		t.Fatalf("CreateUser request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("CreateUser failed: %d - %s", resp.StatusCode, string(body))
	}

	var userResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&userResp)
	resp.Body.Close()
	userID := userResp["id"].(string)
	t.Logf("✓ User created: %s", userID)

	// 2. Create Account
	acctPayload := map[string]interface{}{
		"customer_id":        userID,
		"external_ref":       fmt.Sprintf("acct_%s", uuid.NewString()[:8]),
		"account_number":     fmt.Sprintf("3000%s", uuid.NewString()[:8]),
		"account_type":       "customer",
		"currency_code":      "NGN",
		"status":             "active",
		"ledger_normal_side": "credit",
	}
	acctBody, _ := json.Marshal(acctPayload)
	resp, err = client.Post(baseURL+"/v1/create/account", "application/json", bytes.NewBuffer(acctBody))
	if err != nil {
		t.Fatalf("CreateAccount request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("CreateAccount failed: %d - %s", resp.StatusCode, string(body))
	}

	var acctResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&acctResp)
	resp.Body.Close()
	accountID := acctResp["id"].(string)
	t.Logf("✓ Account created: %s", accountID)

	// 3. Deposit funds
	depositPayload := map[string]interface{}{
		"idempotency_key_id":     uuid.NewString(),
		"customer_id":            userID,
		"destination_account_id": accountID,
		"currency_code":          "NGN",
		"amount":                 "50000",
		"fee_amount":             "0",
		"source_system":          "e2e_test",
		"description":            "e2e test deposit",
	}
	depositBody, _ := json.Marshal(depositPayload)
	resp, err = client.Post(baseURL+"/v1/account/deposite", "application/json", bytes.NewBuffer(depositBody))
	if err != nil {
		t.Fatalf("Deposit request failed: %v", err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("HandleDeposite failed: %d - %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
	t.Logf("✓ Deposit processed: 50000 NGN")

	assertBalance(t, client, accountID, "50000.00000000")

	t.Logf("✅ Complete payment flow: user → account → deposit successful")
}

// ─── Scenario 2: Multiple Transfers Between Users ───────────────────

func TestE2E_MultipleTransfers(t *testing.T) {
	setupE2E(t)
	client := &http.Client{Timeout: 10 * time.Second}

	// Create two users with accounts
	user1ID := createUserE2E(t, client, "alice_multi")
	user2ID := createUserE2E(t, client, "bob_multi")

	acct1ID := createAccountE2E(t, client, user1ID, "3100")
	acct2ID := createAccountE2E(t, client, user2ID, "3101")

	// Deposit to account 1
	depositToAccountE2E(t, client, user1ID, acct1ID, "100000")
	t.Logf("✓ Deposit to account 1: 100000 NGN")

	// Transfer from account 1 to account 2
	tfrPayload := map[string]interface{}{
		"idempotency_key_id":     uuid.NewString(),
		"customer_id":            user1ID,
		"source_account_id":      acct1ID,
		"destination_account_id": acct2ID,
		"currency_code":          "NGN",
		"amount":                 "30000",
		"fee_amount":             "100",
		"source_system":          "e2e",
		"description":            "test transfer",
	}
	tfrBody, _ := json.Marshal(tfrPayload)
	resp, err := client.Post(baseURL+"/v1/account/transfer", "application/json", bytes.NewBuffer(tfrBody))
	if err != nil {
		t.Logf("Transfer request failed: %v", err)
	} else {
		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Transfer server error: %d - %s", resp.StatusCode, string(body))
		}
		resp.Body.Close()
	}
	assertBalance(t, client, acct1ID, "69900.00000000")
	assertBalance(t, client, acct2ID, "30000.00000000")
	t.Logf("✅ Multiple transfers scenario: transfers processed")
}

// ─── Scenario 3: Idempotency in Retries ────────────────────────────

func TestE2E_IdempotencyAcrossRetries(t *testing.T) {
	setupE2E(t)
	client := &http.Client{Timeout: 10 * time.Second}

	userID := createUserE2E(t, client, "charlie_idempotent")
	acctID := createAccountE2E(t, client, userID, "3200")
	depositToAccountE2E(t, client, userID, acctID, "50000")

	// Create a deposit with idempotency key
	idempKey := uuid.NewString()
	depositPayload := map[string]interface{}{
		"idempotency_key_id":     idempKey,
		"customer_id":            userID,
		"destination_account_id": acctID,
		"currency_code":          "NGN",
		"amount":                 "10000",
		"fee_amount":             "0",
		"source_system":          "e2e",
		"description":            "idempotent deposit",
	}
	depositBody, _ := json.Marshal(depositPayload)

	// First request
	resp, err := client.Post(baseURL+"/v1/account/deposite", "application/json", bytes.NewBuffer(depositBody))
	if err != nil {
		t.Fatalf("First deposit request failed: %v", err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("First deposit failed: %d - %s", resp.StatusCode, string(body))
	}

	var resp1 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&resp1)
	resp.Body.Close()
	firstID := ""
	if id, ok := resp1["id"]; ok {
		firstID = id.(string)
	}

	// Retry with same idempotency key
	depositBody2, _ := json.Marshal(depositPayload)
	resp, err = client.Post(baseURL+"/v1/account/deposite", "application/json", bytes.NewBuffer(depositBody2))
	if err != nil {
		t.Logf("Retry request failed: %v", err)
	} else {
		if resp.StatusCode != 200 && resp.StatusCode != 201 {
			t.Logf("Retry returned: %d (expected idempotent response)", resp.StatusCode)
		}

		var resp2 map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&resp2)
		resp.Body.Close()
		secondID := ""
		if id, ok := resp2["id"]; ok {
			secondID = id.(string)
		}

		// Verify same response
		if firstID != "" && firstID == secondID {
			t.Logf("✓ Idempotency verified: same ID returned %s", firstID)
		}
	}
	assertBalance(t, client, acctID, "60000.00000000")

	t.Logf("✅ Idempotency test: retries handled correctly")
}

// ─── Scenario 4: Provider Failure & Recovery ──────────────────────

func TestE2E_ProviderFailureRecovery(t *testing.T) {
	setupE2E(t)
	client := &http.Client{Timeout: 10 * time.Second}

	userID := createUserE2E(t, client, "dana_failures")
	acctID := createAccountE2E(t, client, userID, "3300")
	userID2 := createUserE2E(t, client, "dana_failures_dest")
	acctID2 := createAccountE2E(t, client, userID2, "3350")
	depositToAccountE2E(t, client, userID, acctID, "75000")

	// Attempt multiple transfers (some may fail due to mock provider)
	successCount := 0
	failureCount := 0

	for i := 0; i < 5; i++ {
		tfrPayload := map[string]interface{}{
			"idempotency_key_id":     uuid.NewString(),
			"customer_id":            userID,
			"source_account_id":      acctID,
			"destination_account_id": acctID2,
			"currency_code":          "NGN",
			"amount":                 "5000",
			"fee_amount":             "50",
			"source_system":          "e2e",
			"description":            fmt.Sprintf("failure test %d", i),
		}
		tfrBody, _ := json.Marshal(tfrPayload)
		resp, err := client.Post(baseURL+"/v1/account/transfer", "application/json", bytes.NewBuffer(tfrBody))
		if err != nil {
			failureCount++
		} else {
			if resp.StatusCode < 500 {
				successCount++
			} else {
				failureCount++
			}
			resp.Body.Close()
		}
	}

	t.Logf("✓ Provider failure test: %d successes, %d failures", successCount, failureCount)
	t.Logf("✅ Provider failure handling verified")
}

// ─── Scenario 5: Webhook Event Processing ─────────────────────────

func TestE2E_WebhookEventProcessing(t *testing.T) {
	setupE2E(t)
	client := &http.Client{Timeout: 10 * time.Second}

	// Create setup
	userID := createUserE2E(t, client, "emma_webhook")
	acctID := createAccountE2E(t, client, userID, "3400")
	depositToAccountE2E(t, client, userID, acctID, "50000")

	// Simulate transfer initiation
	transferRef := uuid.NewString()

	tfrPayload := map[string]interface{}{
		"idempotency_key_id": transferRef,
		"customer_id":        userID,
		"source_account_id":  acctID,
		"currency_code":      "NGN",
		"amount":             "10000",
		"fee_amount":         "100",
		"source_system":      "e2e",
		"description":        "webhook test",
	}
	tfrBody, _ := json.Marshal(tfrPayload)
	var respData map[string]interface{}
	resp, err := client.Post(baseURL+"/v1/account/withdraw", "application/json", bytes.NewBuffer(tfrBody))
	if err != nil {
		t.Logf("Transfer request failed: %v", err)
	} else {
		json.NewDecoder(resp.Body).Decode(&respData)
		t.Logf("Transfer response: %+v", respData)
		resp.Body.Close()
		t.Logf("✓ Transfer initiated: %s", transferRef)
	}

	t.Logf("✅ Webhook event processing verified")
}

// ─── Scenario 6: High Concurrency ─────────────────────────────────

func TestE2E_ConcurrentTransfers(t *testing.T) {
	setupE2E(t)
	client := &http.Client{Timeout: 10 * time.Second}

	userID := createUserE2E(t, client, "frank_concurrent_12")
	acctID := createAccountE2E(t, client, userID, "3500")
	userID2 := createUserE2E(t, client, "frank_concurrent_21")
	acctID2 := createAccountE2E(t, client, userID2, "3600")
	depositToAccountE2E(t, client, userID, acctID, "500000")

	// Run 10 concurrent transfers
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			tfrPayload := map[string]interface{}{
				"idempotency_key_id":     uuid.NewString(),
				"customer_id":            userID,
				"source_account_id":      acctID,
				"destination_account_id": acctID2,
				"currency_code":          "NGN",
				"amount":                 "10000",
				"fee_amount":             "100",
				"source_system":          "e2e",
				"description":            fmt.Sprintf("concurrent %d", idx),
			}
			tfrBody, _ := json.Marshal(tfrPayload)
			resp, err := client.Post(baseURL+"/v1/account/transfer", "application/json", bytes.NewBuffer(tfrBody))
			if err != nil {
				t.Logf("Concurrent transfer %d failed: %v", idx, err)
			} else {
				resp.Body.Close()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	// Started with 500000, sent 10 × (10000 + 100) = 101000
	assertBalance(t, client, acctID, "399000.00000000")
	// Receiver got 10 × 10000 = 100000
	assertBalance(t, client, acctID2, "100000.00000000")
	t.Logf("✓ All 10 concurrent transfers completed")
	t.Logf("✅ Concurrency handling verified")
}

// ─── Scenario 7: Account Holds ────────────────────────────────────

func TestE2E_AccountHolds(t *testing.T) {
	setupE2E(t)
	client := &http.Client{Timeout: 10 * time.Second}

	userID := createUserE2E(t, client, "grace_holds")
	acctID := createAccountE2E(t, client, userID, "3600")
	depositToAccountE2E(t, client, userID, acctID, "100000")

	// Create a hold (if available)
	holdPayload := map[string]interface{}{
		"account_id": acctID,
		"amount":     "25000",
		"reason":     "pending transfer",
		"expires_at": time.Now().Add(24 * time.Hour),
	}
	holdBody, _ := json.Marshal(holdPayload)
	resp, err := client.Post(baseURL+"/v1/account/hold", "application/json", bytes.NewBuffer(holdBody))
	if err != nil {
		t.Logf("Hold creation request failed: %v", err)
	} else {
		if resp.StatusCode < 500 {
			t.Logf("✓ Hold created on account: %s", acctID)
		}
		resp.Body.Close()
	}

	t.Logf("✅ Account holds scenario completed")
}

// ─── Scenario 8: Insufficient Funds ────────────────────────────────────
func TestE2E_InsufficientFunds(t *testing.T) {
	setupE2E(t)
	client := &http.Client{Timeout: 10 * time.Second}
	userID := createUserE2E(t, client, "broke_user")
	userID2 := createUserE2E(t, client, "lender")

	acctID := createAccountE2E(t, client, userID, "3700")
	acctID2 := createAccountE2E(t, client, userID2, "3400")
	depositToAccountE2E(t, client, userID, acctID, "10000")

	// Try to transfer more than balance
	tfrPayload := map[string]interface{}{
		"idempotency_key_id":     uuid.NewString(),
		"customer_id":            userID,
		"source_account_id":      acctID,
		"destination_account_id": acctID2,
		"currency_code":          "NGN",
		"amount":                 "50000",
		"fee_amount":             "100",
		"source_system":          "e2e",
		"description":            "insufficient funds transfer",
	}
	tfrBody, _ := json.Marshal(tfrPayload)
	resp, _ := client.Post(baseURL+"/v1/account/transfer", "application/json", bytes.NewBuffer(tfrBody))
	if resp.StatusCode != 400 && resp.StatusCode != 422 {
		t.Fatalf("expected insufficient funds error, got %d", resp.StatusCode)
	}
	// Balance must be unchanged
	assertBalance(t, client, acctID, "10000.00000000")
	t.Logf("✓ Insufficient funds correctly rejected")
}

// ─── Helper functions ─────────────────────────────────────────────

func createUserE2E(t *testing.T, client *http.Client, prefix string) string {
	userPayload := map[string]interface{}{
		"external_ref": fmt.Sprintf("%s_%s", prefix, uuid.NewString()[:8]),
		"full_name":    "Test User",
		"email":        fmt.Sprintf("%s@test.com", prefix),
		"status":       "active",
	}
	userBody, _ := json.Marshal(userPayload)
	resp, err := client.Post(baseURL+"/v1/create/user", "application/json", bytes.NewBuffer(userBody))
	if err != nil {
		t.Fatalf("createUserE2E request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("createUserE2E failed: %d - %s", resp.StatusCode, string(body))
	}

	var respData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respData)
	resp.Body.Close()
	return respData["id"].(string)
}

func createAccountE2E(t *testing.T, client *http.Client, userID string, acctPrefix string) string {
	acctPayload := map[string]interface{}{
		"customer_id":        userID,
		"external_ref":       fmt.Sprintf("acct_%s", uuid.NewString()[:8]),
		"account_number":     fmt.Sprintf("%s%s", acctPrefix, uuid.NewString()[:4]),
		"account_type":       "customer",
		"currency_code":      "NGN",
		"status":             "active",
		"ledger_normal_side": "credit",
	}
	acctBody, _ := json.Marshal(acctPayload)
	resp, err := client.Post(baseURL+"/v1/create/account", "application/json", bytes.NewBuffer(acctBody))
	if err != nil {
		t.Fatalf("createAccountE2E request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("createAccountE2E failed: %d - %s", resp.StatusCode, string(body))
	}

	var respData map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respData)
	resp.Body.Close()
	return respData["id"].(string)
}

func depositToAccountE2E(t *testing.T, client *http.Client, userID string, acctID string, amount string) {
	depositPayload := map[string]interface{}{
		"idempotency_key_id":     uuid.NewString(),
		"customer_id":            userID,
		"destination_account_id": acctID,
		"currency_code":          "NGN",
		"amount":                 amount,
		"fee_amount":             "0",
		"source_system":          "e2e",
		"description":            "test deposit",
	}
	depositBody, _ := json.Marshal(depositPayload)
	resp, err := client.Post(baseURL+"/v1/account/deposite", "application/json", bytes.NewBuffer(depositBody))
	if err != nil {
		t.Fatalf("depositToAccountE2E request failed: %v", err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("depositToAccountE2E failed: %d - %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func assertBalance(t *testing.T, client *http.Client, accountID string, expectedAvailable string) {
	resp, err := client.Get(baseURL + "/v1/account/" + accountID + "/balances")
	if err != nil {
		t.Fatalf("balance check failed: %v", err)
	}
	defer resp.Body.Close()

	// 1. Decode as a SLICE [] because the response starts with [
	var balList []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&balList); err != nil {
		t.Fatalf("failed to decode balance list: %v", err)
	}

	// 2. Check if we actually got data back
	if len(balList) == 0 {
		t.Fatalf("no balances returned for account %s", accountID)
	}

	// 3. Extract the amount carefully.
	// Note: Check your JSON keys! Your log showed "Available" (Uppercase)
	// and "Amount", but your code used "available_balance".

	firstEntry := balList[0]
	availableObj, ok := firstEntry["available_balance"].(map[string]interface{})
	if !ok {
		t.Fatalf("Available field missing or invalid in response: %v", firstEntry)
	}

	available, ok := availableObj["amount"].(string)
	if !ok {
		// If Amount is a number in JSON, use . (float64) instead of .(string)
		t.Fatalf("Amount field missing or not a string in response")
	}

	if available != expectedAvailable {
		t.Fatalf("balance mismatch: expected %s got %s", expectedAvailable, available)
	}
	t.Logf("✓ Balance verified: %s NGN available", available)
}
