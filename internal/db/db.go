package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/config"
	"github.com/mustafa-oezdemir/ecommerce-gin/migrations"
)

var DB *gorm.DB

func Init(cfg *config.Config) error {
	databaseLogger := gormlogger.New(
		slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
		gormlogger.Config{
			SlowThreshold:             500 * time.Millisecond,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  false,
		},
	)
	database, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{TranslateError: true, Logger: databaseLogger})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		return fmt.Errorf("get connection pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DBPingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return fmt.Errorf("ping database: %w", err)
	}
	if err := migrations.Apply(database); err != nil {
		_ = sqlDB.Close()
		return fmt.Errorf("apply migrations: %w", err)
	}
	DB = database
	return nil
}

func SQL() (*sql.DB, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	return DB.DB()
}

func Close() error {
	sqlDB, err := SQL()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
