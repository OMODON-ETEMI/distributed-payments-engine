# E2E Tests (End-to-End)

End-to-end tests for the distributed payments engine that test the **actual running server** via HTTP.

## Test Scenarios

Our E2E test suite covers 7 key scenarios using **real HTTP calls** to an already-running server:

### 1. **Complete Payment Flow**
- User creation → Account creation → Deposit → Balance check
- Tests the full happy path with real HTTP requests
- **Run**: `TestE2E_CompletePaymentFlow`

### 2. **Multiple Transfers Between Users**
- Create multiple users and accounts
- Execute transfers between different accounts  
- Validates multi-user workflows
- **Run**: `TestE2E_MultipleTransfers`

### 3. **Idempotency Across Retries**
- Verify same request with same idempotency key returns cached result
- Prevents duplicate processing
- **Run**: `TestE2E_IdempotencyAcrossRetries`

### 4. **Provider Failure & Recovery**
- Tests with multiple transfer attempts
- Validates system behavior when provider calls fail
- **Run**: `TestE2E_ProviderFailureRecovery`

### 5. **Webhook Event Processing**
- Simulates transfer initiation → webhook callback → settlement
- Tests async event handling with mocked Paystack webhooks
- **Run**: `TestE2E_WebhookEventProcessing`

### 6. **High Concurrency**
- Runs 10 concurrent transfers simultaneously
- Validates thread-safe operations and race condition handling
- **Run**: `TestE2E_ConcurrentTransfers`

### 7. **Account Holds**
- Tests hold creation and account locking mechanisms
- **Run**: `TestE2E_AccountHolds`

## Prerequisites

Before running E2E tests, ensure:

1. **Server is running**:
   ```bash
   # In one terminal
   go run src/main.go
   # Server should be listening at http://localhost:8000
   ```

2. **PostgreSQL is running** (via Docker Compose):
   ```bash
   docker-compose up -d postgres redis
   ```

3. **Redis is running** at `localhost:6379`

4. **`.env` file is configured** with `e2e_TEST_DB_URL`:
   ```env
   e2e_TEST_DB_URL=postgres://payment_user:lendsqr@test-db:5433/distributed_payments_test?sslmode=disable
   API_BASE_URL=http://localhost:8000  # Optional, defaults to http://localhost:8000
   ```

5. **`main.go` file is configured** with `e2e_TEST_DB_URL`:
   ```main.go
   dbUrl := os.Getenv("e2e_TEST_DB_URL")
   ```

## Running E2E Tests

### Run All E2E Tests

```bash
go test -v -tags=e2e ./tests/e2e/...
```

### Run Specific E2E Test

```bash
go test -v -run TestE2E_CompletePaymentFlow -tags=e2e ./tests/e2e/...
```

### Run with Verbose Output & No Cache

```bash
go test -v -count=1 -tags=e2e ./tests/e2e/...
```

### Run with Timeout

```bash
go test -timeout 60s -v -tags=e2e ./tests/e2e/...
```

## Architecture

```
E2E Test Flow:
   setupE2E()  → Verify server is running at API_BASE_URL
              → All tests use http.Client for real HTTP requests
              
Real HTTP requests:
   http.Client.Post() → http://localhost:8000/v1/... → Server Handler → DB → Response
   
Result:
   - Tests the actual running system
   - Tests real HTTP handling (serialization, headers, etc.)
   - Tests the full request/response cycle
```

## Mock Provider in Server

The server running must be configured with `MockProvider` (0% failure rate):

**In `src/main.go`**:
```go
mockProvider := routes.NewMockProvider("paystack", 0.0) // 0% failure for reliable tests
```

The MockProvider:
- Simulates Paystack behavior without real API keys
- Auto-generates fake transfer codes
- Simulates webhook callbacks after 2 seconds
- No external dependencies needed

## Troubleshooting

### "Server not running" error
Ensure your server is running:
```bash
go run src/main.go
# Server should output: listening on :8000
```

### "TEST_DB_URL not set" warning
Add to `.env`:
```env
TEST_DB_URL=postgres://payment_user:lendsqr@localhost:5433/distributed_payments_test?sslmode=disable
```

### Database connection errors
Ensure Docker Compose is running:
```bash
docker-compose ps  # Should show postgres and redis running

# If not, start them:
docker-compose up -d postgres redis
```

### Tests timeout
Increase timeout:
```bash
go test -timeout 120s -v -tags=e2e ./tests/e2e/...
```

## Quick Start

```bash
# Terminal 1: Start the server
go run src/main.go

# Terminal 2: Run Docker services
docker-compose up -d postgres redis

# Terminal 3: Run E2E tests
go test -v -tags=e2e ./tests/e2e/...
```

## Future Enhancements

- [ ] Add real Paystack sandbox integration (when credentials available)
- [ ] Add performance benchmarks (p99 latency, throughput)
- [ ] Add stress testing with 100+ concurrent operations
- [ ] Add audit trail verification
- [ ] Add journal reconciliation checks
- [ ] Add visual metrics/dashboards for test results

```bash
go test -v -run TestE2E_PaystackWebhook -tags=e2e ./tests/e2e/... 
```

## E2E Test Coverage

### Test Scenarios (To Be Implemented)

1. **TestE2E_CompletePaymentFlow**
   - User creation via API
   - Account creation via API
   - Real Paystack transfer initiation
   - Webhook receipt and processing
   - Balance reconciliation

2. **TestE2E_MultiCurrencyTransfer**
   - Multiple currency codes (NGN, USD, GBP)
   - Exchange rate handling
   - Cross-currency settlement
   - Balance consistency validation

3. **TestE2E_RetryAndReconciliation**
   - Failed payment retry
   - Webhook retry handling
   - Balance reconciliation after failures
   - Idempotency key reuse verification

4. **TestE2E_HighThroughputScenario**
   - 100+ concurrent transfers
   - Load balancer behavior
   - Database connection pooling
   - Circuit breaker under stress

5. **TestE2E_DisasterRecovery**
   - Database failover simulation
   - Webhook delivery guarantees
   - Automatic retry logic
   - Data consistency after recovery

## Test Helpers

```go
// Example E2E helper (to be implemented)
func createTestUserWithRealAPI(t *testing.T, apiURL string) string {
    // Call API directly over HTTP
    // Return user ID
}

func initiateRealPaystackTransfer(t *testing.T, transferID string) string {
    // Call Paystack API
    // Return transfer code
}

func waitForWebhook(t *testing.T, transferID string, timeout time.Duration) {
    // Poll database or webhook log
    // Verify webhook was received and processed
}
```

## Debugging E2E Failures

### Check External Service Status

```bash
# Verify API is running
curl -X GET http://localhost:8080/v1/err

# Check Paystack sandbox status
curl -X GET https://api.paystack.co/balance \
  -H "Authorization: Bearer sk_test_xxx"
```

### View Complete Request/Response Logs

```bash
# Enable debug logging
go test -v -run TestE2E_CompletePaymentFlow -tags=e2e -test.v ./tests/e2e/...

# Check application logs
docker logs -f payment_service_engine
```

## CI/CD Integration

### GitHub Actions Example

```yaml
- name: Run E2E Tests
  env:
    PAYSTACK_API_KEY: ${{ secrets.PAYSTACK_SANDBOX_KEY }}
    API_BASE_URL: http://localhost:8080
  run: |
    docker-compose up -d
    sleep 10
    go test -v -tags=e2e ./tests/e2e/...
```

## Performance Baselines (From E2E)

Expected performance metrics:
- User creation: < 50ms
- Account creation: < 75ms
- Deposit: < 200ms
- Transfer initiation: < 150ms
- Webhook processing: < 100ms

## Known Limitations

- E2E tests require external services (Paystack, etc.)
- Slower than integration tests (5-30s per test)
- Not suitable for every commit (run in staging environment only)
- Requires careful cleanup to avoid test data pollution

## Next: Performance Testing

After E2E tests pass, see `tests/performance/README.md` for load testing and performance benchmarking.
