# Performance Tests

Load and performance testing for the distributed payments engine. These tests validate system behavior under high concurrency, measure latency percentiles, and identify bottlenecks.

## Prerequisites

- Integration tests passing
- PostgreSQL 16+ with sufficient resources (4+ CPU, 8GB+ RAM)
- Redis 7+ with sufficient resources
- Go 1.25+
- Load testing tools: `vegeta`, `k6`, or `Apache JMeter` (optional)

## Performance Test Tools

### Built-in Go Testing

```bash
# Run benchmarks
go test -bench=. -benchmem ./tests/performance/...

# Run with CPU profiling
go test -bench=. -cpuprofile=cpu.prof ./tests/performance/...
go tool pprof cpu.prof

# Run with memory profiling
go test -bench=. -memprofile=mem.prof ./tests/performance/...
go tool pprof mem.prof
```

### Vegeta Load Testing

Install:
```bash
go install github.com/tsenart/vegeta@latest
```

Example attack:
```bash
echo "POST http://localhost:8080/v1/create/user" | vegeta attack -duration=30s -rate=100/s | vegeta report
```

### K6 Load Testing

Install:
```bash
# macOS
brew install k6

# Linux
sudo apt-get install k6

# Windows - download from https://github.com/grafana/k6/releases
```

Example script (see `tests/performance/k6-scenario.js`):
```bash
k6 run tests/performance/k6-scenario.js
```

## Running Performance Tests

### Run All Performance Benchmarks

```bash
go test -v -run TestPerf ./tests/performance/... -timeout 120s
```

### Run Specific Performance Test

```bash
go test -v -run TestPerf_CreateUserThroughput ./tests/performance/... -timeout 120s
```

### Run with Memory Profiling

```bash
go test -bench=BenchmarkCreateUser -benchmem -memprofile=mem.prof ./tests/performance/...
go tool pprof -http=:8081 mem.prof
```

### Run with CPU Profiling

```bash
go test -bench=BenchmarkTransfer -cpuprofile=cpu.prof ./tests/performance/...
go tool pprof -http=:8081 cpu.prof
```

## Performance Test Coverage (To Be Implemented)

### Throughput Tests

1. **TestPerf_CreateUserThroughput**
   - Measure: requests/second
   - Target: > 1000 req/s
   - Concurrency: 50 goroutines

2. **TestPerf_CreateAccountThroughput**
   - Measure: requests/second
   - Target: > 800 req/s
   - Concurrency: 50 goroutines

3. **TestPerf_DepositThroughput**
   - Measure: requests/second
   - Target: > 500 req/s
   - Concurrency: 100 goroutines (DB writes are slower)

4. **TestPerf_TransferThroughput**
   - Measure: requests/second
   - Target: > 300 req/s
   - Concurrency: 100 goroutines (complex transaction)

### Latency Tests

5. **BenchmarkCreateUser**
   - Measure: ns/op, allocs/op
   - Target: < 50ms p99

6. **BenchmarkTransfer**
   - Measure: ns/op, allocs/op
   - Target: < 200ms p99

### Stress Tests

7. **TestPerf_SustainedLoad**
   - Duration: 60s
   - Rate: 500 req/s
   - Concurrency: 200 goroutines
   - Validate: no memory leaks, connection pool stability

8. **TestPerf_Spike**
   - Baseline: 100 req/s
   - Spike to: 1000 req/s for 10s
   - Validate: circuit breaker triggers, graceful degradation

9. **TestPerf_DatabaseConnectionPool**
   - Open 100+ concurrent connections
   - Verify: connection pooling limits respected
   - Validate: no connection leaks

### K6 Load Test Scenarios

10. **k6-scenario.js: Realistic User Journey**
    - 30 seconds duration
    - Ramp up: 0 → 100 VUs
    - Stages: Create user, account, deposit, transfer
    - Measure: Success rate, latency percentiles

## Performance Baselines

Target metrics for production readiness:

| Operation | Target P50 | Target P99 | Target Throughput |
|-----------|-----------|-----------|------------------|
| CreateUser | 10ms | 50ms | > 1000 req/s |
| CreateAccount | 15ms | 75ms | > 800 req/s |
| Deposit | 50ms | 200ms | > 500 req/s |
| Transfer | 100ms | 300ms | > 300 req/s |
| Webhook | 20ms | 100ms | > 1000 req/s |

## Example Performance Test Structure

```go
func BenchmarkCreateUser(b *testing.B) {
    setup()
    defer teardown()
    
    for i := 0; i < b.N; i++ {
        payload := map[string]interface{}{
            "external_ref": fmt.Sprintf("user_%d", i),
            "full_name": "Test User",
            "email": fmt.Sprintf("user%d@test.com", i),
            "status": "active",
        }
        body, _ := json.Marshal(payload)
        w := httptest.NewRecorder()
        req := httptest.NewRequest("POST", "/v1/create/user", bytes.NewBuffer(body))
        testAPI.HandleCreateUser(w, req)
        
        if w.Code != 200 {
            b.Fatalf("unexpected status: %d", w.Code)
        }
    }
}

func TestPerf_CreateUserThroughput(t *testing.T) {
    setup(t)
    start := time.Now()
    var wg sync.WaitGroup
    results := make(chan int, 1000)
    
    for i := 0; i < 1000; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            // Create user request
            results <- 1
        }(i)
    }
    
    wg.Wait()
    close(results)
    
    count := 0
    for range results {
        count++
    }
    
    elapsed := time.Since(start)
    throughput := float64(count) / elapsed.Seconds()
    t.Logf("Throughput: %.0f req/s", throughput)
    
    if throughput < 500 {
        t.Fatalf("throughput too low: %.0f req/s (expected > 500)", throughput)
    }
}
```

## Profiling and Optimization

### CPU Profiling

```bash
go test -bench=BenchmarkTransfer -cpuprofile=cpu.prof ./tests/performance/...
go tool pprof -http=:8081 cpu.prof

# Look for:
# - Hot functions (>10% CPU)
# - Unnecessary allocations
# - Database query time
# - JSON marshaling overhead
```

### Memory Profiling

```bash
go test -bench=BenchmarkTransfer -memprofile=mem.prof ./tests/performance/...
go tool pprof -alloc_space -http=:8081 mem.prof

# Look for:
# - Large allocations
# - Memory leaks (alloc_live)
# - Unnecessary string/slice copies
```

### Database Query Analysis

```sql
-- Find slow queries
SELECT query, mean_exec_time, max_exec_time, calls 
FROM pg_stat_statements 
ORDER BY mean_exec_time DESC 
LIMIT 20;

-- Check table sizes
SELECT schemaname, tablename, pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) 
FROM pg_tables 
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

## Optimization Strategies

1. **Caching**: Implement Redis caching for frequently queried data
2. **Batching**: Batch database operations where possible
3. **Connection Pooling**: Tune PostgreSQL and application connection pool sizes
4. **Indexing**: Add database indexes on frequently queried columns
5. **Query Optimization**: Use EXPLAIN ANALYZE to optimize slow queries
6. **Async Processing**: Move non-critical operations to async workers

## CI/CD Integration

### GitHub Actions Performance Test

```yaml
- name: Run Performance Tests
  run: |
    docker-compose up -d
    sleep 20
    go test -bench=. -benchmem -timeout=300s ./tests/performance/...
    
- name: Upload Results
  uses: actions/upload-artifact@v3
  with:
    name: performance-results
    path: benchstat.txt
```

### Compare Performance Between Commits

```bash
# Run baseline
go test -bench=. -benchmem ./tests/performance/... > baseline.txt

# Make changes

# Run new version
go test -bench=. -benchmem ./tests/performance/... > new.txt

# Compare
benchstat baseline.txt new.txt
```

## Expected Performance Improvements

Current metrics → Target metrics:

| Scenario | Current | Target | Improvement |
|----------|---------|--------|-------------|
| CreateUser | 20ms | 10ms | 2x faster |
| Transfer | 300ms | 100ms | 3x faster |
| Webhook | 50ms | 20ms | 2.5x faster |

## Known Bottlenecks (To Investigate)

1. Database connection pool exhaustion
2. JSON marshaling/unmarshaling overhead
3. UUID parsing on every request
4. Lack of query result caching
5. Inefficient transaction scoping

## Next Steps

After performance tests:
1. Identify top 3 bottlenecks
2. Profile with CPU/memory profiling tools
3. Implement targeted optimizations
4. Re-run tests to measure improvements
5. Document performance tuning decisions

See project wiki for optimization details.
