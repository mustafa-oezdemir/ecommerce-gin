package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv        string
	AppPort       string
	GinMode       string
	MySQLHost     string
	MySQLPort     string
	MySQLDatabase string
	MySQLUser     string
	MySQLPassword string
	SessionSecret string
	SessionSecure bool
	DSN           string
}

func Load() *Config {
	_ = godotenv.Load()
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	appPort := strings.TrimSpace(os.Getenv("APP_PORT"))
	ginMode := strings.TrimSpace(os.Getenv("GIN_MODE"))
	mysqlHost := strings.TrimSpace(os.Getenv("MYSQL_HOST"))
	mysqlPort := strings.TrimSpace(os.Getenv("MYSQL_PORT"))
	mysqlDB := strings.TrimSpace(os.Getenv("MYSQL_DATABASE"))
	mysqlUser := strings.TrimSpace(os.Getenv("MYSQL_USER"))
	mysqlPassword := strings.TrimSpace(os.Getenv("MYSQL_PASSWORD"))
	sessionSecret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	sessionSecureValue := strings.TrimSpace(os.Getenv("SESSION_SECURE"))

	if appEnv == "" {
		log.Fatal("APP_ENV is required (development, test, or production)")
	}
	appEnv = strings.ToLower(appEnv)
	if appEnv != "development" && appEnv != "test" && appEnv != "production" {
		log.Fatal("APP_ENV must be one of: development, test, production")
	}

	if appPort == "" {
		log.Fatal("APP_PORT is required")
	}

	if ginMode == "" {
		if appEnv == "production" {
			log.Fatal("GIN_MODE is required in production")
		}
		ginMode = "debug"
	}
	ginMode = strings.ToLower(ginMode)
	if appEnv == "production" && ginMode != "release" {
		log.Fatal("GIN_MODE must be release in production")
	}

	if mysqlHost == "" || mysqlPort == "" || mysqlDB == "" || mysqlUser == "" || mysqlPassword == "" {
		log.Fatal("Required MySQL environment variables are missing: MYSQL_HOST, MYSQL_PORT, MYSQL_DATABASE, MYSQL_USER, MYSQL_PASSWORD")
	}

	if appEnv == "production" && sessionSecret == "" {
		log.Fatal("SESSION_SECRET is required in production")
	}

	sessionSecure, err := strconv.ParseBool(sessionSecureValue)
	if err != nil {
		if appEnv == "production" {
			log.Fatal("SESSION_SECURE must be a boolean value in production")
		}
		sessionSecure = false
	}
	if appEnv == "production" && !sessionSecure {
		log.Fatal("SESSION_SECURE must be true in production")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", mysqlUser, mysqlPassword, mysqlHost, mysqlPort, mysqlDB)

	return &Config{
		AppEnv:        appEnv,
		AppPort:       appPort,
		GinMode:       ginMode,
		MySQLHost:     mysqlHost,
		MySQLPort:     mysqlPort,
		MySQLDatabase: mysqlDB,
		MySQLUser:     mysqlUser,
		MySQLPassword: mysqlPassword,
		SessionSecret: sessionSecret,
		SessionSecure: sessionSecure,
		DSN:           dsn,
	}
}
