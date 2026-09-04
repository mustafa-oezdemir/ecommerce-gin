FROM golang:1.26 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY web ./web

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates \
    && addgroup -S app && adduser -S app -G app \
    && mkdir -p /app/logs && chown app:app /app/logs

COPY --from=builder /out/server /app/server
COPY --from=builder /out/seed /app/seed
RUN chmod 0755 /app/server /app/seed

USER app

EXPOSE 8080

CMD ["/app/server"]
