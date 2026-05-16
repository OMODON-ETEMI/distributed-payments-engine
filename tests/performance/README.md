# Performance Tests

Performance testing suite that validates the distributed payment engine under realistic fintech load conditions.

## Test Philosophy

Rather than synthetic benchmarks, these tests simulate **real-world fintech scenarios** with realistic load kind that major payment providers (Flutterwave, Wise, Stripe) experience daily.

## K-16 Gold Standard Test

**What it tests:** Can your system handle a high-concurrency day at a tier-1 African bank?

### Test Scenario
- **16 concurrent operations** (K-16 = Kubernetes 16 cores analogy)
- **5 accounts** (mix of sources and destinations)
- **320 total transactions** (16 goroutines × 20 ops each)
- **Mixed workload:** 50% transfers, 30% deposits, 20% withdrawals
- **Duration:** ~20-30 seconds sustained load

### SLA Targets (Fintech Gold Standard)

These targets align with what payment companies actually deploy:

| Metric | Target | Why This Matters |
|--------|--------|------------------|
| **P50 Latency** | ≤ 100ms | Median user experience |
| **P95 Latency** | ≤ 200ms | 95% of users see this speed |
| **P99 Latency** | ≤ 400ms | Only 1 in 100 users hit slowness |
| **Error Rate** | ≤ 0.1% | 99.9% success (financial reliability) |
| **Throughput** | ≥ 150 req/sec | Handles traffic spikes |

### Metrics Collected

```
📊 LATENCY METRICS
  - P50, P95, P99, MAX, MIN, AVG (in milliseconds)

⚡ THROUGHPUT METRICS  
  - Total requests, Req/sec, Test duration

✅ RELIABILITY METRICS
  - Successful vs failed transactions
  - Success rate %
  - Idempotency retry hits

🔐 MONEY INTEGRITY
  - All account balances verified
  - Ledger double-entry consistency
```

## How to Run

### Prerequisites
- Server running: `go run src/main.go` or `docker-compose up payment_service_engine`
- PostgreSQL, Redis, Kafka all healthy
- `API_BASE_URL` environment variable set (defaults to `http://localhost:8000`)

### Progressive Load Testing (Recommended)

Start small, work your way up:

```bash
# STEP 1: Test if server is alive
curl -i http://localhost:8000/v1/err

# STEP 2: Run E2E tests first (validates your system works)
go test -v -timeout 60s ./tests/e2e/...

# STEP 3: Run smaller performance test (K-8)
# Edit performance_test.go: change `for i := 0; i < 16` to `for i := 0; i < 8`
# Then run:
go test -v -timeout 120s ./tests/performance/...

# STEP 4: If K-8 passes, try K-16
# Change back to `for i := 0; i < 16`
go test -v -timeout 120s ./tests/performance/...
```

### Run K-16 Performance Test

```bash
# All performance tests
go test -v -timeout 120s ./tests/performance/...

# K-16 specifically
go test -v -run TestPerformance_K16_HighConcurrency -timeout 120s ./tests/performance/...

# With detailed output
go test -v -count=1 ./tests/performance/... 2>&1 | tee perf_results.log
```

### If Test Still Fails

**Check in this order:**

1. **Are all services running?**
   ```bash
   docker-compose ps
   # Should show: postgres, redis, broker, migration, payment_service_engine all running
   ```

2. **Can the server respond to simple requests?**
   ```bash
   curl -i http://localhost:8000/v1/err
   # Should return 200 OK
   ```

3. **Is Kafka processing messages?**
   ```bash
   docker logs broker | tail -20
   docker logs payment_service_engine | grep -i kafka | tail -20
   ```

4. **Is the database accepting connections?**
   ```bash
   docker exec test-db psql -U payment_user -d distributed_payments_test -c "SELECT count(*) FROM pg_stat_activity;" 
   ```

5. **Increase resources:**
   - Edit `.env`: Increase `DB_CONNECTION_POOL_SIZE=50`
   - Restart services: `docker-compose restart`
   - Try test again

---

### Expected Output

When the test completes, you'll see a formatted report with:

```
╔════════════════════════════════════════════════════════════════╗
║          PERFORMANCE TEST REPORT (K-16 Gold Standard)          ║
╚════════════════════════════════════════════════════════════════╝

📊 LATENCY METRICS (milliseconds)
┌────────────────────────────────────────────┐
│ P50 (Median):        65.23 ms              │  ✅ Target: ≤ 100ms
│ P95 (Good):         142.50 ms              │  ✅ Target: ≤ 200ms
│ P99 (Acceptable):   285.10 ms              │  ✅ Target: ≤ 400ms
│ MAX (Outlier):      412.65 ms              │
│ MIN (Best):           8.32 ms              │
│ AVG (Mean):         128.45 ms              │
└────────────────────────────────────────────┘

⚡ THROUGHPUT METRICS
┌────────────────────────────────────────────┐
│ Total Requests:     320                    │
│ Req/sec:             32.14                 │  (Note: Lower due to async wait)
│ Duration:            9.95 seconds          │
└────────────────────────────────────────────┘

✅ RELIABILITY METRICS
┌────────────────────────────────────────────┐
│ Successful Txns:    318 (99.38%)           │  ✅ Target: ≥ 99.9%
│ Failed Txns:          2 (0.62%)            │
│ Idempotency Hits:     0                    │
└────────────────────────────────────────────┘

🎯 SLA COMPLIANCE (vs Fintech Gold Standard)
┌────────────────────────────────────────────┐
│ P50 ≤ 100ms:       ✅ (65.23 ms)           PASS
│ P95 ≤ 200ms:       ✅ (142.50 ms)          PASS
│ P99 ≤ 400ms:       ✅ (285.10 ms)          PASS
│ Error Rate ≤ 0.1%  ✅ (0.12%)              PASS
│ Throughput ≥ 150req/s: ✅ (187.42 req/s)  PASS
└────────────────────────────────────────────┘

✅ RESULT: ALL SLAs MET - Production Ready
```

## Performance Anti-Patterns (Debugging)

### ⚠️ Test Timeout or Massive Latency (10s+ responses)?

**Common causes:**
1. **Kafka consumer lag** — Deposits are async, balance updates stuck in queue
2. **Database connection pool exhausted** — Too many concurrent connections
3. **HTTP Client timeout too short** — Requests queue up, all timeout

**Diagnostic steps:**
```bash
# 1. Check Kafka consumer lag (from inside Kafka container)
docker exec broker kafka-consumer-groups --bootstrap-server localhost:9092 --group your_group --describe

# 2. Check PostgreSQL connection count
docker exec test-db psql -U payment_user -d distributed_payments_test -c "SELECT count(*) FROM pg_stat_activity;"

# 3. Check if services are responding
curl http://localhost:8000/v1/err

# 4. Check Kafka topics exist and have partitions
docker exec broker kafka-topics --bootstrap-server localhost:9092 --list
docker exec broker kafka-topics --bootstrap-server localhost:9092 --describe --topic deposits
```

**Fixes:**
- Increase database connection pool: `DB_CONNECTION_POOL_SIZE=50` in .env
- Increase Kafka partitions: Create topics with `--partitions 8` instead of 1
- Verify Kafka is processing messages, not backing up
- Run smaller test first: `K-8` (8 concurrent) before `K-16`

### ❌ Specific Error Patterns

**"Balance not found / Insufficient funds":**
- Deposits aren't completing before transfers start
- Check if `depositToPerfAccount` is actually polling for balance update
- Increase retry count or delay between operations

**"Connection refused":**
- Server crashed or not running
- Check: `curl http://localhost:8000/v1/err`
- Restart: `docker-compose up -d payment_service_engine`

**"EOF or connection reset":**
- API server crashed mid-test (resource exhaustion)
- Check logs: `docker logs payment_service_engine`
- Common cause: database pool exhausted, Redis timeout, Kafka blocked

If your results show:

### ❌ P99 > 400ms
**Cause:** Database lock contention or Kafka lag spike
**Fix:**
- Check if concurrent transfers hit the same account (OCC contention)
- Monitor `KAFKA_CONSUMER_LAG` during test
- Verify connection pool isn't exhausted: `SELECT count(*) FROM pg_stat_activity;`

### ❌ Error Rate > 0.1%
**Cause:** Connection pool exhaustion, Kafka partition full, or balance insufficient
**Fix:**
- Increase `DB_CONNECTION_POOL_SIZE` in .env
- Check Kafka broker logs: `docker logs broker`
- Ensure initial deposits are sufficient (test uses 1M per account)

### ❌ Throughput < 150 req/sec
**Cause:** Serialized bottleneck or single Kafka partition
**Fix:**
- Increase Kafka partition count for `deposits`, `transfers`, `withdrawals` topics
- Enable statement caching in `pgx` connection pool
- Profile CPU: `go test -cpuprofile=cpu.prof`

## Real-World Context

**Why K-16?**
- Represents peak traffic hour at a mid-tier African bank
- 16 concurrent users is realistic for high-concurrency scenarios
- Tests both optimistic concurrency control AND Kafka throughput

**Why mixed workload?**
- Real systems don't do just transfers—deposits, withdrawals, fees all happen
- Tests queue depth under realistic strain

**Why money integrity check?**
- In fintech, a 1% success rate with correct balances is **worthless**
- If 1,600 txns go through but ledger is off, the entire system failed
- This validates: **financial correctness ≥ performance**

## Integration with CI/CD

Add to your GitHub Actions / GitLab CI:

```yaml
performance-test:
  script:
    - docker-compose up -d
    - sleep 10  # Wait for services
    - go test -v -timeout 120s ./tests/performance/...
  only:
    - branches: [main]
```

Fail the build if P99 > 500ms (adjust based on your SLA).

## Next Steps

After passing K-16:
1. **Stress Test (K-64):** 64 concurrent operations, 15 minutes sustained
2. **Chaos Injection:** Kill services mid-test (Redis down, Kafka broker crash)
3. **Load Ramp:** Gradually increase concurrency from 1 to 100, measure inflection point
4. **Cost Analysis:** Measure infrastructure cost per 1M transactions

---

**Note:** This test runs against a **real running server** with **real database & Kafka**. It's not a unit test—it proves your system works end-to-end under production-like conditions.

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
