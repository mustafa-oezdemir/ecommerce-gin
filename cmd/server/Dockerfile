FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -o /out/server \
    ./cmd/server

RUN CGO_ENABLED=0 GOOS=linux go build \
    -o /out/seed \
    ./cmd/seed


FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/server /app/server
COPY --from=builder /out/seed /app/seed

EXPOSE 8080

CMD ["/app/server"]