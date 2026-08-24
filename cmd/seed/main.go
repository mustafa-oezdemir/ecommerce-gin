package main

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// User model (projendeki şemaya göre uyarlayabilirsin)
type User struct {
	ID       uint   `gorm:"primaryKey"`
	Name     string `gorm:"size:100;not null"`
	Email    string `gorm:"size:150;uniqueIndex;not null"`
	Password string `gorm:"size:255;not null"`
	Role     string `gorm:"size:50;not null"`
}

func hashPassword(pw string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("bcrypt error: %v", err)
	}
	return string(h)
}

func getDSN() string {
	// Öncelik: MYSQL_DSN varsa onu kullan
	if dsn := os.Getenv("MYSQL_DSN"); dsn != "" {
		return dsn
	}
	// Aksi halde parçaları kullanarak oluştur
	user := os.Getenv("MYSQL_USER")
	pass := os.Getenv("MYSQL_PASSWORD")
	host := os.Getenv("MYSQL_HOST")
	port := os.Getenv("MYSQL_PORT")
	db := os.Getenv("MYSQL_DATABASE")
	if user == "" || pass == "" || host == "" || port == "" || db == "" {
		log.Fatal("Missing DB env vars. Set MYSQL_DSN or MYSQL_USER, MYSQL_PASSWORD, MYSQL_HOST, MYSQL_PORT, MYSQL_DATABASE")
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, pass, host, port, db)
}

func main() {
	dsn := getDSN()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}

	// Tablo yoksa oluştur
	if err := db.AutoMigrate(&User{}); err != nil {
		log.Fatalf("migrate error: %v", err)
	}

	// Seed verileri
	users := []User{
		{Name: "Admin User", Email: "admin@example.com", Password: hashPassword("AdminPass123!"), Role: "admin"},
		{Name: "Employee User", Email: "employee@example.com", Password: hashPassword("EmployeePass123!"), Role: "employee"},
		{Name: "Customer User", Email: "customer@example.com", Password: hashPassword("CustomerPass123!"), Role: "customer"},
	}

	for _, u := range users {
		var existing User
		err := db.Where("email = ?", u.Email).First(&existing).Error
		if err == nil {
			log.Printf("user %s already exists, skipping", u.Email)
			continue
		}
		if err := db.Create(&u).Error; err != nil {
			log.Fatalf("create user error: %v", err)
		}
		log.Printf("created user %s with role %s", u.Email, u.Role)
	}
}
