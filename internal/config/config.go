package config

import (
    "fmt"
    "log"
    "os"

    "github.com/joho/godotenv"
)

type Config struct {
    DSN string
}

func Load() *Config {
    // Try to load .env from current working dir and parent (helps when running from cmd/server)
    _ = godotenv.Load()
    _ = godotenv.Load("../.env")

    user := os.Getenv("MYSQL_USER")
    pass := os.Getenv("MYSQL_PASSWORD")
    host := os.Getenv("MYSQL_HOST")
    port := os.Getenv("MYSQL_PORT")
    db := os.Getenv("MYSQL_DATABASE")

    if user == "" || pass == "" || host == "" || port == "" || db == "" {
        log.Fatal("One or more required MySQL env vars are missing (MYSQL_USER, MYSQL_PASSWORD, MYSQL_HOST, MYSQL_PORT, MYSQL_DATABASE)")
    }

    dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, pass, host, port, db)
    return &Config{DSN: dsn}
}
