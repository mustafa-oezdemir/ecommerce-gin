package config

import (
	"crypto/sha256"
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
	AppEnv                string
	AppPort               string
	MetricsPort           string
	GinMode               string
	TrustedProxies        []string
	LogLevel              string
	LogConsoleFormat      string
	LogFile               string
	LogMaxSizeMB          int
	LogMaxBackups         int
	LogMaxAgeDays         int
	LogCompress           bool
	LogAddSource          bool
	HTTPReadHeaderTimeout time.Duration
	HTTPReadTimeout       time.Duration
	HTTPWriteTimeout      time.Duration
	HTTPIdleTimeout       time.Duration
	HTTPShutdownTimeout   time.Duration
	HTTPMaxHeaderBytes    int
	ProductImageDirectory string
	ProductImageMaxBytes  int64
	ProductImageMaxWidth  int
	ProductImageMaxHeight int
	ProductImageMaxPixels int64
	ClamAVAddress         string
	ClamAVScanTimeout     time.Duration
	MySQLHost             string
	MySQLPort             string
	MySQLDatabase         string
	MySQLUser             string
	MySQLPassword         string
	DBMaxOpenConns        int
	DBMaxIdleConns        int
	DBConnMaxLifetime     time.Duration
	DBConnMaxIdleTime     time.Duration
	DBConnectTimeout      time.Duration
	DBReadTimeout         time.Duration
	DBWriteTimeout        time.Duration
	DBPingTimeout         time.Duration
	SessionSecret         string
	SessionSecure         bool
	CSRFKey               []byte
	SecurityEncryptionKey []byte
	DSN                   string
}

func Load() *Config {
	_ = godotenv.Load()
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	appPort := strings.TrimSpace(os.Getenv("APP_PORT"))
	metricsPort := strings.TrimSpace(os.Getenv("METRICS_PORT"))
	ginMode := strings.TrimSpace(os.Getenv("GIN_MODE"))
	trustedProxies := envCSV("TRUSTED_PROXIES")
	logLevel := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	logConsoleFormat := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_CONSOLE_FORMAT")))
	logFile := strings.TrimSpace(os.Getenv("LOG_FILE"))
	logMaxSizeMB := envInt("LOG_MAX_SIZE_MB", 100)
	logMaxBackups := envInt("LOG_MAX_BACKUPS", 5)
	logMaxAgeDays := envInt("LOG_MAX_AGE_DAYS", 28)
	logCompress := envBool("LOG_COMPRESS", true)
	logAddSource := envBool("LOG_ADD_SOURCE", false)
	httpReadHeaderTimeout := envDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second)
	httpReadTimeout := envDuration("HTTP_READ_TIMEOUT", 15*time.Second)
	httpWriteTimeout := envDuration("HTTP_WRITE_TIMEOUT", 30*time.Second)
	httpIdleTimeout := envDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	httpShutdownTimeout := envDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second)
	httpMaxHeaderBytes := envInt("HTTP_MAX_HEADER_BYTES", 1<<20)
	productImageDirectory := strings.TrimSpace(os.Getenv("PRODUCT_IMAGE_DIRECTORY"))
	productImageMaxBytes := envInt64("PRODUCT_IMAGE_MAX_BYTES", 5<<20)
	productImageMaxWidth := envInt("PRODUCT_IMAGE_MAX_WIDTH", 6000)
	productImageMaxHeight := envInt("PRODUCT_IMAGE_MAX_HEIGHT", 6000)
	productImageMaxPixels := envInt64("PRODUCT_IMAGE_MAX_PIXELS", 25_000_000)
	clamAVAddress := strings.TrimSpace(os.Getenv("CLAMAV_ADDRESS"))
	clamAVScanTimeout := envDuration("CLAMAV_SCAN_TIMEOUT", 15*time.Second)
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
	securitySecret := strings.TrimSpace(os.Getenv("SECURITY_ENCRYPTION_KEY"))

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
	if logLevel == "" {
		logLevel = "info"
	}
	if logLevel != "debug" && logLevel != "info" && logLevel != "warn" && logLevel != "error" {
		log.Fatal("LOG_LEVEL must be one of: debug, info, warn, error")
	}
	if logConsoleFormat == "" {
		if appEnv == "production" {
			logConsoleFormat = "json"
		} else {
			logConsoleFormat = "text"
		}
	}
	if logConsoleFormat != "text" && logConsoleFormat != "json" {
		log.Fatal("LOG_CONSOLE_FORMAT must be one of: text, json")
	}
	if logFile == "" {
		logFile = "logs/ecommerce.log"
	}
	if logMaxSizeMB < 1 || logMaxBackups < 1 || logMaxAgeDays < 1 {
		log.Fatal("LOG_MAX_SIZE_MB, LOG_MAX_BACKUPS, and LOG_MAX_AGE_DAYS must be at least 1")
	}
	if httpMaxHeaderBytes < 8192 {
		log.Fatal("HTTP_MAX_HEADER_BYTES must be at least 8192")
	}
	if productImageDirectory == "" {
		productImageDirectory = "uploads/products"
	}
	if productImageMaxBytes < 1024 || productImageMaxWidth < 1 || productImageMaxHeight < 1 || productImageMaxPixels < 1 {
		log.Fatal("product image size and dimension limits must be positive")
	}
	if clamAVAddress == "" {
		clamAVAddress = "127.0.0.1:3310"
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
	securityKey, err := base64.StdEncoding.DecodeString(securitySecret)
	if err != nil || len(securityKey) != 32 {
		if appEnv == "production" {
			log.Fatal("SECURITY_ENCRYPTION_KEY must be valid base64 and decode to exactly 32 bytes")
		}
		fallback := sha256.Sum256(append([]byte("pehlione-account-security:"), csrfKey...))
		securityKey = fallback[:]
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
		AppEnv:                appEnv,
		AppPort:               appPort,
		MetricsPort:           metricsPort,
		GinMode:               ginMode,
		TrustedProxies:        trustedProxies,
		LogLevel:              logLevel,
		LogConsoleFormat:      logConsoleFormat,
		LogFile:               logFile,
		LogMaxSizeMB:          logMaxSizeMB,
		LogMaxBackups:         logMaxBackups,
		LogMaxAgeDays:         logMaxAgeDays,
		LogCompress:           logCompress,
		LogAddSource:          logAddSource,
		HTTPReadHeaderTimeout: httpReadHeaderTimeout,
		HTTPReadTimeout:       httpReadTimeout,
		HTTPWriteTimeout:      httpWriteTimeout,
		HTTPIdleTimeout:       httpIdleTimeout,
		HTTPShutdownTimeout:   httpShutdownTimeout,
		HTTPMaxHeaderBytes:    httpMaxHeaderBytes,
		ProductImageDirectory: productImageDirectory,
		ProductImageMaxBytes:  productImageMaxBytes,
		ProductImageMaxWidth:  productImageMaxWidth,
		ProductImageMaxHeight: productImageMaxHeight,
		ProductImageMaxPixels: productImageMaxPixels,
		ClamAVAddress:         clamAVAddress,
		ClamAVScanTimeout:     clamAVScanTimeout,
		MySQLHost:             mysqlHost,
		MySQLPort:             mysqlPort,
		MySQLDatabase:         mysqlDB,
		MySQLUser:             mysqlUser,
		MySQLPassword:         mysqlPassword,
		DBMaxOpenConns:        dbMaxOpenConns,
		DBMaxIdleConns:        dbMaxIdleConns,
		DBConnMaxLifetime:     dbConnMaxLifetime,
		DBConnMaxIdleTime:     dbConnMaxIdleTime,
		DBConnectTimeout:      dbConnectTimeout,
		DBReadTimeout:         dbReadTimeout,
		DBWriteTimeout:        dbWriteTimeout,
		DBPingTimeout:         dbPingTimeout,
		SessionSecret:         sessionSecret,
		SessionSecure:         sessionSecure,
		CSRFKey:               csrfKey,
		SecurityEncryptionKey: securityKey,
		DSN:                   dsn,
	}
}

func envCSV(name string) []string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
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

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
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

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatalf("%s must be a boolean", name)
	}
	return parsed
}
