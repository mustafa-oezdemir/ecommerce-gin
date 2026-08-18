# Place this Dockerfile at the project root (same level as go.mod)
FROM golang:1.26

WORKDIR /app

# Copy go.mod and go.sum first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the project
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server

EXPOSE 8080

CMD ["/app/server"]
