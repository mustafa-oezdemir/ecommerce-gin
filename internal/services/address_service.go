package services

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

var (
	ErrInvalidAddress  = errors.New("invalid address")
	ErrAddressNotFound = errors.New("address not found")
	countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)
)

type AddressService struct{ database *gorm.DB }

func NewAddressService(database *gorm.DB) *AddressService { return &AddressService{database: database} }

func (s *AddressService) List(ctx context.Context, userID uint) ([]models.UserAddress, error) {
	if userID == 0 {
		return nil, ErrInvalidUser
	}
	var addresses []models.UserAddress
	err := s.database.WithContext(ctx).Where("user_id = ?", userID).Order("is_default DESC, created_at ASC").Find(&addresses).Error
	return addresses, err
}

func (s *AddressService) Create(ctx context.Context, userID uint, address models.UserAddress) (*models.UserAddress, error) {
	if userID == 0 || !validAddress(address) {
		return nil, ErrInvalidAddress
	}
	normalizeAddress(&address)
	address.UserID = userID
	address.ID = 0
	address.CreatedAt = address.CreatedAt.UTC()
	err := s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if address.IsDefault {
			if err := tx.Model(&models.UserAddress{}).Where("user_id = ?", userID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&address).Error
	})
	return &address, err
}

func (s *AddressService) Update(ctx context.Context, userID, addressID uint, address models.UserAddress) error {
	if userID == 0 || addressID == 0 || !validAddress(address) {
		return ErrInvalidAddress
	}
	normalizeAddress(&address)
	return s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.UserAddress
		if err := tx.Where("id = ? AND user_id = ?", addressID, userID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAddressNotFound
			}
			return err
		}
		if address.IsDefault {
			if err := tx.Model(&models.UserAddress{}).Where("user_id = ? AND id <> ?", userID, addressID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Model(&current).Updates(map[string]any{
			"first_name": address.FirstName, "last_name": address.LastName, "company": address.Company,
			"street": address.Street, "house_number": address.HouseNumber, "address_line_2": address.AddressLine2,
			"postal_code": address.PostalCode, "city": address.City, "state": address.State,
			"country_code": address.CountryCode, "phone": address.Phone, "is_default": address.IsDefault,
		}).Error
	})
}

func (s *AddressService) Delete(ctx context.Context, userID, addressID uint) error {
	if userID == 0 || addressID == 0 {
		return ErrInvalidAddress
	}
	result := s.database.WithContext(ctx).Where("id = ? AND user_id = ?", addressID, userID).Delete(&models.UserAddress{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAddressNotFound
	}
	return nil
}

func validAddress(address models.UserAddress) bool {
	fields := []struct {
		value string
		max   int
	}{
		{address.FirstName, 100}, {address.LastName, 100}, {address.Company, 150}, {address.Street, 150},
		{address.HouseNumber, 30}, {address.AddressLine2, 150}, {address.PostalCode, 20}, {address.City, 100},
		{address.State, 100}, {address.Phone, 32},
	}
	for index, field := range fields {
		length := utf8.RuneCountInString(strings.TrimSpace(field.value))
		optional := index == 2 || index == 5 || index == 8
		if length > field.max || (!optional && length == 0) {
			return false
		}
	}
	return countryCodePattern.MatchString(strings.ToUpper(strings.TrimSpace(address.CountryCode)))
}

func normalizeAddress(address *models.UserAddress) {
	address.FirstName = strings.TrimSpace(address.FirstName)
	address.LastName = strings.TrimSpace(address.LastName)
	address.Company = strings.TrimSpace(address.Company)
	address.Street = strings.TrimSpace(address.Street)
	address.HouseNumber = strings.TrimSpace(address.HouseNumber)
	address.AddressLine2 = strings.TrimSpace(address.AddressLine2)
	address.PostalCode = strings.TrimSpace(address.PostalCode)
	address.City = strings.TrimSpace(address.City)
	address.State = strings.TrimSpace(address.State)
	address.CountryCode = strings.ToUpper(strings.TrimSpace(address.CountryCode))
	address.Phone = strings.TrimSpace(address.Phone)
}
