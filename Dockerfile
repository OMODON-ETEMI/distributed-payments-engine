FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y \
    build-essential \
    pkg-config \
    librdkafka-dev

WORKDIR /app

RUN go install github.com/air-verse/air@latest
RUN go install github.com/swaggo/swag/cmd/swag@latest

COPY go.mod go.sum ./
RUN go mod download

RUN ls -la /app

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o main ./src/main.go


# --- Final Stage (Production Image) ---
# FROM debian:bookworm-slim
# RUN apt-get update && apt-get install -y ca-certificates librdkafka1 && rm -rf /var/lib/apt/lists/*
# COPY --from=builder /app/main /main
# CMD ["/main"]