package models

import "time"

type ProductList struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	UserID    uint              `gorm:"not null;uniqueIndex:idx_product_lists_user_name"`
	Name      string            `gorm:"size:100;not null;uniqueIndex:idx_product_lists_user_name"`
	SystemKey *string           `gorm:"size:32;uniqueIndex:idx_product_lists_user_system"`
	Items     []ProductListItem `gorm:"foreignKey:ProductListID"`
}

type ProductListItem struct {
	ID            uint `gorm:"primaryKey"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ProductListID uint    `gorm:"not null;uniqueIndex:idx_product_list_item"`
	ProductID     uint    `gorm:"not null;uniqueIndex:idx_product_list_item"`
	Product       Product `gorm:"constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT;"`
}
