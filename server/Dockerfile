FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /stratum-api ./cmd/api

FROM alpine:3.22

RUN apk add --no-cache ca-certificates git && \
    adduser -D -u 1000 stratum

COPY --from=builder /stratum-api /usr/local/bin/stratum-api

USER stratum
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

ENTRYPOINT ["stratum-api"]
