package models

import "gorm.io/gorm"

type Role string

const (
    RoleAdmin    Role = "admin"
    RoleEmployee Role = "employee"
    RoleCustomer Role = "customer"
)

type User struct {
    gorm.Model
    Name     string `gorm:"size:100"`
    Email    string `gorm:"uniqueIndex;size:100"`
    Password string `gorm:"size:255"`
    Role     Role   `gorm:"size:20"`
}
