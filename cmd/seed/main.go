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

	appdb.Init(cfg)

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

	category := models.Category{Name: "Elektronik", Description: "Demo elektronik ürünleri"}
	if err := appdb.DB.Where("name = ?", category.Name).FirstOrCreate(&category).Error; err != nil {
		log.Fatalf("seed category failed: %v", err)
	}
	products := []models.Product{
		{Name: "Demo Laptop", Description: "Geliştirme için demo ürün", PriceCents: 99999, Stock: 10, Active: true, CategoryID: &category.ID},
		{Name: "Demo Kulaklık", Description: "Geliştirme için demo ürün", PriceCents: 4999, Stock: 25, Active: true, CategoryID: &category.ID},
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
