package db

import (
    "log"

    "gorm.io/driver/mysql"
    "gorm.io/gorm"

    "github.com/mustafa-oezdemir/ecommerce-gin/internal/config"
    "github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

var DB *gorm.DB

func Init(cfg *config.Config) {
    var err error
    DB, err = gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{})
    if err != nil {
        log.Fatalf("db connect error: %v", err)
    }

    // Auto-migrate
    err = DB.AutoMigrate(
        &models.User{},
        &models.Product{},
        &models.Order{},
        &models.OrderItem{},
        &models.Cart{},
        &models.CartItem{},
    )
    if err != nil {
        log.Fatalf("migration error: %v", err)
    }
}
