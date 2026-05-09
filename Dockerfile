FROM golang:1.26.1-alpine AS builder

WORKDIR /app

RUN go install github.com/air-verse/air@latest
RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN apt-get update && apt-get install -y \
    build-essential \
    pkg-config \
    librdkafka-dev

COPY go.mod go.sum ./
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux go build -o main ./cmd/api

COPY . .

RUN go build -o main ./src/main.go