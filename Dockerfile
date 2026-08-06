FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/telegram-assistant .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 1000 -S app \
    && adduser -u 1000 -S app -G app

WORKDIR /app
COPY --from=builder /out/telegram-assistant /app/telegram-assistant

USER app
VOLUME ["/app/logs"]

ENTRYPOINT ["/app/telegram-assistant"]
