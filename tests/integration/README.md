# Integration Tests

Production-ready integration tests for the distributed payments engine. These tests validate end-to-end workflows including user creation, account management, deposits, transfers, withdrawals, and webhook handling.

## Prerequisites

- PostgreSQL 16+ running locally or via Docker
- Redis 7+ running locally or via Docker  
- Go 1.25+

## Setup

### 1. Start PostgreSQL and Redis with Docker Compose

```bash
docker-compose up postgres redis -d
```

### 2. Create Test Database

```bash
createdb -U postgres -W distributed_payments_test
```

Or via psql:
```bash
psql -U postgres
CREATE DATABASE distributed_payments_test;
\q
```

### 3. Run Migrations on Test DB

```bash
migrate -path src/database/migrations -database "postgres://postgres:yourpassword@localhost:5432/distributed_payments_test?sslmode=disable" up
```

### 4. Set Test Environment Variables

Create a `.env.test` file:

```env
TEST_DB_URL=postgres://postgres:yourpassword@localhost:5432/distributed_payments_test?sslmode=disable
REDIS_TEST_URL=redis://localhost:6379
PORT=8080
```

Or export directly:

```bash
export TEST_DB_URL="postgres://postgres:yourpassword@localhost:5432/distributed_payments_test?sslmode=disable"
```

## Running Integration Tests

### Run All Integration Tests

```bash
go test -v ./tests/integration/...
```

### Run Specific Integration Test

```bash
go test -v -run TestIntegration_FullUserAccountWorkflow ./tests/integration/...
```

### Run with Coverage

```bash
go test -v -coverprofile=integration_coverage.out ./tests/integration/...
go tool cover -html=integration_coverage.out
```

### Run with Verbose Output and Timeout

```bash
go test -v -timeout 60s ./tests/integration/...
```

## Integration Test Coverage

### Test Suites

1. **TestIntegration_FullUserAccountWorkflow** 
   - User creation
   - Account creation
   - Balance queries
   - Deposit operations
   - Validates end-to-end user onboarding

2. **TestIntegration_TransferBetweenAccounts**
   - Multi-user scenario
   - Multiple account creation
   - Peer-to-peer transfer initiation
   - Validates transfer routing and account isolation

3. **TestIntegration_WebhookProcessing**
   - Simulates Paystack webhook for transfer success
   - Tests webhook signature validation
   - Tests event routing (success/failure)
   - Validates webhook idempotency

4. **TestIntegration_IdempotencyKeyEnforcement**
   - Duplicate request handling
   - Cached response verification
   - Validates idempotency contract
   - Tests Redis-backed idempotency

5. **TestIntegration_ConcurrentRequests**
   - 10 concurrent user creation requests
   - Race condition testing
   - Database transaction isolation
   - Validates concurrent access safety

6. **TestIntegration_ErrorHandling**
   - Invalid UUID handling
   - Non-existent resource lookups
   - Malformed request handling
   - Validates error responses and status codes

## Key Testing Patterns

### Database State Isolation

Each test has access to the full database. For isolation, consider:

```bash
# Reset test database before test run
psql -U postgres -d distributed_payments_test -c "TRUNCATE TABLE customers, accounts, balances, transfer_requests CASCADE;"
```

### Mock Payment Provider

Tests use `MockProvider` with 0% failure rate. For testing provider failures, adjust:

```go
mockProvider := routes.NewMockProvider("paystack", 0.5) // 50% failure rate
```

### Redis for Idempotency

Redis is required for idempotency key locks. If Redis is unavailable, tests will skip gracefully.

## Expected Results

When all prerequisites are met:

```
PASS: TestIntegration_FullUserAccountWorkflow     1.234s
PASS: TestIntegration_TransferBetweenAccounts     0.897s
PASS: TestIntegration_WebhookProcessing           0.456s
PASS: TestIntegration_IdempotencyKeyEnforcement   0.678s
PASS: TestIntegration_ConcurrentRequests          1.123s
PASS: TestIntegration_ErrorHandling               0.234s

ok      github.com/.../tests/integration     4.622s
```

## Debugging Failed Tests

### View Full Database State

```bash
psql -U postgres -d distributed_payments_test
\d  # List all tables
SELECT * FROM customers LIMIT 5;
SELECT * FROM transfer_requests WHERE status = 'pending';
```

### Check Redis State

```bash
redis-cli
KEYS idem:*
GET idem:user_id:key
```

### Enable Verbose Logging

```bash
go test -v -run TestIntegration_FullUserAccountWorkflow ./tests/integration/... -test.v
```

## Performance Considerations

- Each test creates new users and accounts (no cleanup by default)
- Concurrent test creates 10 goroutines
- Expected total test runtime: 4-6 seconds with Docker
- For CI/CD, consider parallel execution: `go test -parallel 4 ./tests/integration/...`

## Next Steps: E2E Testing

After integration tests pass, E2E tests should:
- Use actual Paystack sandbox credentials
- Deploy to staging environment
- Test full external provider integration
- Validate production data flows

See `tests/e2e/README.md` for E2E test setup.

## Next Steps: Performance Testing

After E2E tests pass, performance tests should:
- Load test with 1000+ concurrent users
- Measure p99 latency for critical paths
- Validate database query performance under load
- Test circuit breaker behavior

See `tests/performance/README.md` for performance test setup.
