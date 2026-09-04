package models

import (
	"time"

	"gorm.io/gorm"
)

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleEmployee Role = "employee"
	RoleCustomer Role = "customer"
)

type User struct {
	gorm.Model
	Name                 string `gorm:"size:100"`
	FirstName            string `gorm:"size:100;not null;default:''"`
	LastName             string `gorm:"size:100;not null;default:''"`
	Email                string `gorm:"uniqueIndex;size:254"`
	Password             string `gorm:"size:255"`
	Role                 Role   `gorm:"size:20"`
	SecurityVersion      uint64 `gorm:"not null;default:1"`
	TwoFactorEnabled     bool   `gorm:"not null;default:false"`
	TwoFactorSecret      []byte `gorm:"type:varbinary(512)"`
	TwoFactorConfirmedAt *time.Time
}
