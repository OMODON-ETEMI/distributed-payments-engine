----
----
----
----
----
----
----
----
----
----
----
----
----
----
----
----
----
---------------------|---------|----------|---------|---------|
---------------------|---------|----------|---------|---------|
---------------------|---------|----------|---------|---------|
----
----
----
----
----
----
----
----
----
# 🚀 Distributed Payments & Ledger Engine

> **Architected for Integrity.** A high-performance, financial-grade distributed payment engine written in Go. This project is a comprehensive demonstration of solving the complex consistency, reliability, and scaling challenges inherent in modern fintech infrastructure.

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-316192?style=for-the-badge&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D?style=for-the-badge&logo=redis&logoColor=white)](https://redis.io/)
[![Kafka](https://img.shields.io/badge/Kafka-Streaming-231F20?style=for-the-badge&logo=apachekafka&logoColor=white)](https://kafka.apache.org/)

---

## 💡 The "Why"

In fintech, "eventual consistency" is often a risk. This engine is built on the principle of **strict financial integrity**. It doesn't just "update a balance"—it manages a double-entry ledger that accounts for every kobo. Whether it's a P2P transfer, a withdrawal with a hold, or an external provider integration, the system ensures that **money is never lost and never created out of thin air.**

---

## 🏗️ Core Engineering Patterns

This isn't just a CRUD app. It’s a distributed system solving hard problems:

### 1. 📖 Double-Entry Accounting
Every movement of value is recorded as a `JournalTransaction` with balanced `JournalLines` (Debits vs Credits).
*   **Auditability:** Every balance is a projection of immutable transaction logs.
*   **Integrity:** The system validates that `Sum(Debits) - Sum(Credits) == 0` before any commit.

### 2. 🛡️ The Transactional Outbox Pattern
To solve the "Dual-Write" problem (updating the DB and notifying Kafka), I use the **Outbox Pattern**. 
*   Events are saved to the database in the *same* transaction as the financial update.
*   Background workers use PostgreSQL `LISTEN/NOTIFY` to ship events to Kafka with **at-least-once delivery guarantees**.

### 3. ⚡ Optimistic Concurrency Control (OCC)
Balance updates use version-based locking. If two transfers hit the same account simultaneously, the database ensures only one succeeds, preventing race conditions without the performance penalty of heavy table locks.

### 4. 🔗 Multi-Layer Idempotency
Using a hybrid approach of **Redis (distributed locking)** and **Postgres (unique constraints)**, the engine guarantees that retried requests (e.g., due to a timeout) never result in duplicate charges.

### 5. 🔌 Resilient External Integrations
Integrated with a Mock Paystack Provider protected by a **Circuit Breaker** (using `gobreaker`). If the payment provider goes down, the engine trips the breaker to prevent resource exhaustion and allow for graceful recovery.

---

## ✨ Key Features

*   **Comprehensive Transfers:** P2P, Deposits, and Withdrawals.
*   **Funds Reservation (Holds):** Supports two-stage withdrawals—place a hold, call the provider, then consume or release the hold based on the outcome.
*   **Internal Webhooks:** Secure webhook processing with signature verification and idempotency.
*   **Background Workers:** Scalable workers for outbox event shipping and webhook processing.
*   **Swagger Documentation:** Fully documented API accessible via `/swagger/`.

---

## 🛠️ Tech Stack

*   **Language:** Go (Golang)
*   **Router:** Chi (Lightweight & Fast)
*   **Database:** PostgreSQL (with `pgx` for high-performance pooling)
*   **Cache:** Redis (Idempotency locks)
*   **Messaging:** Apache Kafka (Event streaming)
*   **Tooling:** SQLC (Type-safe SQL), Swagger (Documentation)

---

## 🚀 Getting Started

### Prerequisites
*   Go 1.25+
*   PostgreSQL 16+
*   Redis 7+
*   Kafka (Optional for local development, required for event streaming)

### Installation

1. **Clone & Install**
   ```bash
   git clone https://github.com/OMODON-ETEMI/Oritsetemi-Omodon-lendsqr-be-test.git
   cd distributed-payments-engine
   go mod download
   ```

2. **Configure Environment**
   Create a `.env` file based on `.env.example`:
   ```env
   PORT=8080
   DB_URL=postgres://user:pass@localhost:5432/payments_db
   REDIS_ADDR=localhost:6379
   KAFKA_BROKER=localhost:9092
   ```

3. **Run Migrations**
   ```bash
   # Using your preferred migration tool
   migrate -path database/migrations -database "$DB_URL" up
   ```

4. **Start the Engine**
   ```bash
   go run src/main.go
   ```

---

## 🧪 Testing Strategy

I take testing seriously. The project includes a tiered testing approach:

*   **Unit Tests:** Business logic and utility validation.
*   **Integration Tests:** Real DB/Redis workflows (User -> Account -> Transfer).
*   **E2E Tests:** Real HTTP calls simulating the full lifecycle of a payment.
*   **Performance Tests:** Benchmark and stress testing for high-concurrency scenarios.

```bash
# Run all tests
go test ./...

# Run E2E tests specifically
go test -v -tags=e2e ./tests/e2e/...
```

---

## 📂 Project Structure

```text
src/
├── database/          # SQLC generated code and DB helpers
├── internal/
│   ├── messaging/     # Kafka producers/consumers
│   ├── outbox/        # Outbox pattern implementation
│   └── worker/        # Background notification listeners
├── routes/            # HTTP Handlers, Middleware, and Router
└── main.go            # Application Entry Point
tests/
├── e2e/               # HTTP Flow tests
├── integration/       # Database workflow tests
└── unit/              # Logic tests
```

---

## 👨‍💻 Let's Connect

I'm a **Backend Engineer** obsessed with building systems that don't fail when it matters most. I specialize in distributed architectures, transaction integrity, and high-performance Go services. 

**I am currently open to new opportunities.** If you are looking for an engineer who treats every line of code as a liability and every transaction as sacred, let's talk.

**Oritsetemi Omodon**  
*   📧 [etemiomodon@gmail.com](mailto:etemiomodon@gmail.com)
*   💼 [LinkedIn](https://linkedin.com/in/oritsetemi-omodon)
*   🐙 [GitHub](https://github.com/OMODON-ETEMI)

---

<div align="center">

**"Code is cheap, but financial integrity is priceless."**  
Built with ❤️ and a lot of Go.

</div>
