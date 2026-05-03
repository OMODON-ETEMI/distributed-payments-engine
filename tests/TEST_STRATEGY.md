# Testing Strategy & Roadmap

Complete testing framework for the distributed payments engine. This document outlines the test pyramid, running instructions, and best practices.

## Test Pyramid

```
                    ▲
                   ╱ ╲
                  ╱   ╲        Performance Tests
                 ╱     ╲       (Stress, Load, Benchmarks)
                ╱───────╲      ~5 tests, ~300s total
               ╱ E2E     ╲
              ╱  Tests    ╲    End-to-End Tests
             ╱─────────────╲   (Real services, Paystack, etc)
            ╱ Integration   ╲   ~10 tests, ~30s total
           ╱    Tests        ╲
          ╱───────────────────╲
         │   Unit Tests        │  Unit Tests
         │   (Utilities, ...)  │  ~50 tests, ~2s total
         └─────────────────────┘
```

## Testing Levels

### 1. Unit Tests (Fast, Isolated)
- **Location**: `tests/unit/`
- **Focus**: Utility functions, JSON helpers, provider logic
- **Database**: None (mocked)
- **Speed**: 1-2 seconds total
- **Run**: `go test -v ./tests/unit/...`
- **Coverage Target**: 80%+ of non-DB code

### 2. Integration Tests (Medium Speed, Real DB)
- **Location**: `tests/integration/`
- **Focus**: Complete workflows (user → account → transfer)
- **Database**: Real PostgreSQL
- **Redis**: Real Redis (idempotency)
- **Speed**: 4-6 seconds total
- **Run**: `go test -v ./tests/integration/...`
- **Coverage Target**: All routes, handlers

### 3. E2E Tests (Slow, Real Services)
- **Location**: `tests/e2e/`
- **Focus**: Full system with external providers (Paystack)
- **Database**: Real PostgreSQL (staging)
- **Services**: Real Paystack sandbox
- **Speed**: 30s-2m per test
- **Run**: `go test -v -tags=e2e ./tests/e2e/...`
- **Coverage Target**: Business flows, payment scenarios

### 4. Performance Tests (Varies)
- **Location**: `tests/performance/`
- **Focus**: Throughput, latency, stress testing
- **Database**: Real PostgreSQL
- **Concurrency**: 50-200 goroutines
- **Speed**: 30s-5m per test
- **Run**: `go test -bench=. ./tests/performance/...`
- **Targets**: Latency p99, throughput, memory

## Quick Start

### Run All Tests

```bash
# Unit tests (always fast)
go test -v ./tests/unit/...

# Integration tests (requires DB + Redis)
go test -v ./tests/integration/...

# E2E tests (requires staging environment)
go test -v -tags=e2e ./tests/e2e/...

# Performance tests (requires resources)
go test -bench=. ./tests/performance/...

# All together
go test -v ./...
```

### Run by Category

```bash
# All fast tests (unit + integration)
go test -v -short ./tests/...

# Only integration
go test -v -run Integration ./tests/integration/...

# Only performance
go test -v -bench=. -run=^$ ./tests/performance/...
```

### With Coverage

```bash
# Unit test coverage
go test -v -cover ./tests/unit/...

# Generate coverage HTML
go test -coverprofile=coverage.out ./tests/unit/...
go tool cover -html=coverage.out
```

## Test Execution Matrix

| Test Level | When to Run | Duration | Environment | DB Required |
|------------|------------|----------|-------------|-------------|
| Unit | Every commit | 1-2s | Local | No |
| Integration | Pre-merge | 5-10s | Local or CI | Yes |
| E2E | After merge | 1-5m | Staging | Yes |
| Performance | Weekly | 5-30m | Dedicated | Yes |

## Running Tests in CI/CD

### GitHub Actions Example

```yaml
name: Tests

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: 1.25
      - run: go test -v ./tests/unit/...

  integration-tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_DB: distributed_payments_test
          POSTGRES_PASSWORD: postgres
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      redis:
        image: redis:7-alpine
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: 1.25
      - run: |
          export TEST_DB_URL="postgres://postgres:postgres@localhost:5432/distributed_payments_test?sslmode=disable"
          go test -v ./tests/integration/...

  performance-tests:
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: 1.25
      - run: go test -bench=. -timeout=300s ./tests/performance/...
```

## Best Practices

### 1. Test Data Isolation
```go
// Create unique identifiers for each test run
testID := uuid.NewString()
userRef := fmt.Sprintf("test_user_%s", testID)
```

### 2. Cleanup After Tests
```go
defer func() {
    // Clean up resources
    testRedis.FlushDB()
    connPool.Close()
}()
```

### 3. Use Table-Driven Tests
```go
tests := []struct {
    name    string
    input   string
    wantErr bool
}{
    {"valid uuid", "00000000-0000-0000-0000-000000000000", false},
    {"invalid uuid", "not-a-uuid", true},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // test logic
    })
}
```

### 4. Use Helper Functions
```go
func createTestUser(t *testing.T, api *routes.ApiConfig) string {
    // Common user creation logic
    // Return user ID
}
```

### 5. Parallel Test Execution
```bash
go test -v -parallel 4 ./tests/integration/...
```

### 6. Verbose Output
```bash
go test -v -count=1 ./tests/integration/...
```

## Debugging Failed Tests

### View Test Output

```bash
# Verbose mode
go test -v -run TestIntegration_FullUserAccountWorkflow ./tests/integration/...

# Keep test files after run
go test -v -keepfail ./tests/integration/...
```

### Check Database State

```bash
psql -U postgres -d distributed_payments_test -c "\dt"
psql -U postgres -d distributed_payments_test -c "SELECT * FROM customers LIMIT 5;"
```

### Check Redis State

```bash
redis-cli
KEYS idem:*
GET idem:user_id:key
FLUSHDB
```

### Enable Debug Logging

```go
// In test file
import "log"

func init() {
    log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// In tests
log.Printf("User created: %s", userID)
```

## Test Coverage Goals

| Level | Current | Target |
|-------|---------|--------|
| Unit | 60% | 80% |
| Integration | 40% | 70% |
| E2E | 0% | 50% |
| Overall | 35% | 75% |

Track coverage:
```bash
go test -v -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1
```

## Performance Benchmarks

Run benchmarks and compare:

```bash
# First run
go test -bench=. -benchmem ./tests/performance/... > before.txt

# After optimizations
go test -bench=. -benchmem ./tests/performance/... > after.txt

# Compare
benchstat before.txt after.txt
```

## Common Issues & Solutions

| Issue | Solution |
|-------|----------|
| "database: cannot connect" | Start PostgreSQL: `docker-compose up postgres` |
| "redis: connection refused" | Start Redis: `docker-compose up redis` |
| "TEST_DB_URL not set" | Export: `export TEST_DB_URL=...` |
| "port 8080 already in use" | Change PORT env var or kill process |
| "test timeout" | Increase timeout: `go test -timeout=300s ...` |

## Future Improvements

- [ ] Add contract testing for API versioning
- [ ] Implement chaos engineering tests
- [ ] Add database backup/restore testing
- [ ] Create performance comparison CI job
- [ ] Add synthetic monitoring tests
- [ ] Implement canary deployment tests

## Documentation

- [Unit Tests](tests/unit/README.md) - Utility and handler unit tests
- [Integration Tests](tests/integration/README.md) - Database workflows
- [E2E Tests](tests/e2e/README.md) - External service integration
- [Performance Tests](tests/performance/README.md) - Load and stress testing

## Support

For test failures or questions:
1. Check test logs with `-v` flag
2. Review test file comments
3. Check service status (DB, Redis)
4. Enable debug logging
5. Review PR comments for known issues
