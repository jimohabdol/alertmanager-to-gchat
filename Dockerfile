FROM golang:1.24.3-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY config.toml ./

RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o alertmanager-to-gchat

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/alertmanager-to-gchat .
COPY --from=builder /app/config.toml .

EXPOSE 7000

ENTRYPOINT ["/app/alertmanager-to-gchat"]
CMD ["--config", "/app/config.toml"]