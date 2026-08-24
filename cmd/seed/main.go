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
