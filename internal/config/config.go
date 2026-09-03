package config

import (
	"encoding/base64"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv            string
	AppPort           string
	MetricsPort       string
	GinMode           string
	MySQLHost         string
	MySQLPort         string
	MySQLDatabase     string
	MySQLUser         string
	MySQLPassword     string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	DBConnMaxIdleTime time.Duration
	DBConnectTimeout  time.Duration
	DBReadTimeout     time.Duration
	DBWriteTimeout    time.Duration
	DBPingTimeout     time.Duration
	SessionSecret     string
	SessionSecure     bool
	CSRFKey           []byte
	DSN               string
}

func Load() *Config {
	_ = godotenv.Load()
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	appPort := strings.TrimSpace(os.Getenv("APP_PORT"))
	metricsPort := strings.TrimSpace(os.Getenv("METRICS_PORT"))
	ginMode := strings.TrimSpace(os.Getenv("GIN_MODE"))
	mysqlHost := strings.TrimSpace(os.Getenv("MYSQL_HOST"))
	mysqlPort := strings.TrimSpace(os.Getenv("MYSQL_PORT"))
	mysqlDB := strings.TrimSpace(os.Getenv("MYSQL_DATABASE"))
	mysqlUser := strings.TrimSpace(os.Getenv("MYSQL_USER"))
	mysqlPassword := strings.TrimSpace(os.Getenv("MYSQL_PASSWORD"))
	dbMaxOpenConns := envInt("DB_MAX_OPEN_CONNS", 25)
	dbMaxIdleConns := envInt("DB_MAX_IDLE_CONNS", 10)
	dbConnMaxLifetime := envDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute)
	dbConnMaxIdleTime := envDuration("DB_CONN_MAX_IDLE_TIME", time.Minute)
	dbConnectTimeout := envDuration("DB_CONNECT_TIMEOUT", 5*time.Second)
	dbReadTimeout := envDuration("DB_READ_TIMEOUT", 10*time.Second)
	dbWriteTimeout := envDuration("DB_WRITE_TIMEOUT", 10*time.Second)
	dbPingTimeout := envDuration("DB_PING_TIMEOUT", 5*time.Second)
	sessionSecret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	sessionSecureValue := strings.TrimSpace(os.Getenv("SESSION_SECURE"))
	csrfSecret := strings.TrimSpace(os.Getenv("CSRF_SECRET"))

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
	if metricsPort == "" {
		metricsPort = "9091"
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
	if dbMaxOpenConns < 1 {
		log.Fatal("DB_MAX_OPEN_CONNS must be at least 1")
	}
	if dbMaxIdleConns < 0 || dbMaxIdleConns > dbMaxOpenConns {
		log.Fatal("DB_MAX_IDLE_CONNS must be between 0 and DB_MAX_OPEN_CONNS")
	}

	if len(sessionSecret) < 32 {
		log.Fatal("SESSION_SECRET must be at least 32 characters")
	}

	csrfKey, err := base64.StdEncoding.DecodeString(csrfSecret)
	if err != nil || len(csrfKey) != 32 {
		log.Fatal("CSRF_SECRET must be valid base64 and decode to exactly 32 bytes")
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

	mysqlConfig := mysqldriver.Config{
		User:                 mysqlUser,
		Passwd:               mysqlPassword,
		Net:                  "tcp",
		Addr:                 net.JoinHostPort(mysqlHost, mysqlPort),
		DBName:               mysqlDB,
		Params:               map[string]string{"charset": "utf8mb4", "collation": "utf8mb4_unicode_ci"},
		ParseTime:            true,
		Loc:                  time.Local,
		Timeout:              dbConnectTimeout,
		ReadTimeout:          dbReadTimeout,
		WriteTimeout:         dbWriteTimeout,
		AllowNativePasswords: true,
	}
	dsn := mysqlConfig.FormatDSN()

	return &Config{
		AppEnv:            appEnv,
		AppPort:           appPort,
		MetricsPort:       metricsPort,
		GinMode:           ginMode,
		MySQLHost:         mysqlHost,
		MySQLPort:         mysqlPort,
		MySQLDatabase:     mysqlDB,
		MySQLUser:         mysqlUser,
		MySQLPassword:     mysqlPassword,
		DBMaxOpenConns:    dbMaxOpenConns,
		DBMaxIdleConns:    dbMaxIdleConns,
		DBConnMaxLifetime: dbConnMaxLifetime,
		DBConnMaxIdleTime: dbConnMaxIdleTime,
		DBConnectTimeout:  dbConnectTimeout,
		DBReadTimeout:     dbReadTimeout,
		DBWriteTimeout:    dbWriteTimeout,
		DBPingTimeout:     dbPingTimeout,
		SessionSecret:     sessionSecret,
		SessionSecure:     sessionSecure,
		CSRFKey:           csrfKey,
		DSN:               dsn,
	}
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("%s must be an integer", name)
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		log.Fatalf("%s must be a positive duration (for example 5s or 1m)", name)
	}
	return parsed
}
