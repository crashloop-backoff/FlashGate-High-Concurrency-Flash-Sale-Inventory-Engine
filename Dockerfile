# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/flashgate-api ./cmd/api

FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 appgroup \
    && adduser -S -u 10001 -G appgroup appuser

WORKDIR /app

COPY --from=builder /out/flashgate-api /app/flashgate-api

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/flashgate-api"]
