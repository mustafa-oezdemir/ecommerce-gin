package main

import (
	"errors"
	"log"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/config"
	appdb "github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type seedUser struct {
	Name     string
	Email    string
	Password string
	Role     models.Role
}

func main() {
	cfg := config.Load()
	if cfg.AppEnv == "production" {
		log.Fatal("seed is disabled in production")
	}

	if err := appdb.Init(cfg); err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer func() {
		if err := appdb.Close(); err != nil {
			log.Printf("database close failed: %v", err)
		}
	}()

	users := []seedUser{
		{
			Name:     "Admin User",
			Email:    "admin@example.com",
			Password: "AdminPass123!",
			Role:     models.RoleAdmin,
		},
		{
			Name:     "Employee User",
			Email:    "employee@example.com",
			Password: "EmployeePass123!",
			Role:     models.RoleEmployee,
		},
		{
			Name:     "Customer User",
			Email:    "customer@example.com",
			Password: "CustomerPass123!",
			Role:     models.RoleCustomer,
		},
	}

	for _, user := range users {
		if err := createUserIfNotExists(appdb.DB, user); err != nil {
			log.Fatalf("seed user %s failed: %v", user.Email, err)
		}
	}
	if err := translateLegacyDemoData(appdb.DB); err != nil {
		log.Fatalf("translate legacy demo data failed: %v", err)
	}

	category := models.Category{Name: "Electronics", Description: "Demo electronics products"}
	if err := appdb.DB.Where("name = ?", category.Name).FirstOrCreate(&category).Error; err != nil {
		log.Fatalf("seed category failed: %v", err)
	}
	products := []models.Product{
		{Name: "Demo Laptop", Description: "Demo product for development", PriceCents: 99999, Stock: 10, Active: true, CategoryID: &category.ID},
		{Name: "Demo Headphones", Description: "Demo product for development", PriceCents: 4999, Stock: 25, Active: true, CategoryID: &category.ID},
	}
	for _, product := range products {
		var existing models.Product
		if err := appdb.DB.Where("name = ?", product.Name).First(&existing).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			if err := appdb.DB.Create(&product).Error; err != nil {
				log.Fatalf("seed product %s failed: %v", product.Name, err)
			}
		} else if err != nil {
			log.Fatalf("seed product lookup %s failed: %v", product.Name, err)
		}
	}

	log.Println("seed completed successfully")
}

func translateLegacyDemoData(database *gorm.DB) error {
	if err := database.Model(&models.Category{}).
		Where("name = ?", "Elektronik").
		Updates(map[string]any{"name": "Electronics", "description": "Demo electronics products"}).Error; err != nil {
		return err
	}
	if err := database.Model(&models.Product{}).
		Where("name = ?", "Demo Laptop").
		Update("description", "Demo product for development").Error; err != nil {
		return err
	}
	return database.Model(&models.Product{}).
		Where("name = ?", "Demo Kulakl\u0131k").
		Updates(map[string]any{"name": "Demo Headphones", "description": "Demo product for development"}).Error
}

func createUserIfNotExists(database *gorm.DB, seed seedUser) error {
	var existing models.User

	err := database.
		Where("email = ?", seed.Email).
		First(&existing).
		Error

	if err == nil {
		log.Printf("user %s already exists, skipping", seed.Email)
		return nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(seed.Password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return err
	}

	user := models.User{
		Name:     seed.Name,
		Email:    seed.Email,
		Password: string(hashedPassword),
		Role:     seed.Role,
	}

	if err := database.Create(&user).Error; err != nil {
		return err
	}

	log.Printf(
		"created user %s with role %s",
		user.Email,
		user.Role,
	)

	return nil
}
