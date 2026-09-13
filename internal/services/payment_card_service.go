package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/banking"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

var (
	ErrPaymentCardUnavailable = errors.New("payment card service unavailable")
	ErrPaymentCardInvalid     = errors.New("invalid payment card")
	ErrPaymentCardNotFound    = errors.New("payment card not found")
)

type AddPaymentCardInput struct {
	UserID     uint
	CardNumber string
	ExpMonth   int
	ExpYear    int
	CVC        string
	IsDefault  bool
	RequestID  string
}

type PaymentCardService struct {
	database *gorm.DB
	banking  banking.CardClient
}

func NewPaymentCardService(database *gorm.DB, client banking.CardClient) *PaymentCardService {
	return &PaymentCardService{database: database, banking: client}
}

func (service *PaymentCardService) Enabled() bool { return service != nil && service.banking != nil }

func (service *PaymentCardService) List(ctx context.Context, userID uint) ([]models.SavedPaymentCard, error) {
	if service == nil || service.database == nil || userID == 0 {
		return nil, ErrPaymentCardNotFound
	}
	var cards []models.SavedPaymentCard
	err := service.database.WithContext(ctx).Where("user_id = ?", userID).Order("is_default DESC, created_at DESC").Find(&cards).Error
	return cards, err
}

func (service *PaymentCardService) Get(ctx context.Context, userID, cardID uint) (*models.SavedPaymentCard, error) {
	if service == nil || service.database == nil || userID == 0 || cardID == 0 {
		return nil, ErrPaymentCardNotFound
	}
	var card models.SavedPaymentCard
	if err := service.database.WithContext(ctx).Where("id = ? AND user_id = ?", cardID, userID).First(&card).Error; err != nil {
		return nil, ErrPaymentCardNotFound
	}
	return &card, nil
}

func (service *PaymentCardService) Add(ctx context.Context, input AddPaymentCardInput) (*models.SavedPaymentCard, error) {
	now := time.Now().UTC()
	if !service.Enabled() || input.UserID == 0 || input.ExpMonth < 1 || input.ExpMonth > 12 || input.ExpYear < now.Year() || (input.ExpYear == now.Year() && input.ExpMonth < int(now.Month())) || len(strings.TrimSpace(input.CVC)) != 3 {
		return nil, ErrPaymentCardInvalid
	}
	tokenized, err := service.banking.TokenizeCard(ctx, banking.TokenizeCardRequest{
		CardNumber: input.CardNumber, ExpMonth: input.ExpMonth, ExpYear: input.ExpYear, CVC: input.CVC, RequestID: input.RequestID,
	})
	if err != nil {
		return nil, ErrPaymentCardInvalid
	}
	card := models.SavedPaymentCard{UserID: input.UserID, ProviderToken: tokenized.Token, Brand: tokenized.Brand, Last4: tokenized.Last4, ExpMonth: tokenized.ExpMonth, ExpYear: tokenized.ExpYear, IsDefault: input.IsDefault}
	err = service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.SavedPaymentCard
		lookup := tx.Where("user_id = ? AND brand = ? AND last4 = ? AND exp_month = ? AND exp_year = ?", input.UserID, card.Brand, card.Last4, card.ExpMonth, card.ExpYear).First(&existing)
		if lookup.Error == nil {
			card.IsDefault = existing.IsDefault || input.IsDefault
			if input.IsDefault {
				if err := tx.Model(&models.SavedPaymentCard{}).Where("user_id = ? AND id <> ?", input.UserID, existing.ID).Update("is_default", false).Error; err != nil {
					return err
				}
			}
			card.ID = existing.ID
			return tx.Model(&existing).Updates(map[string]any{"provider_token": card.ProviderToken, "is_default": card.IsDefault}).Error
		}
		if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return lookup.Error
		}
		if !input.IsDefault {
			var count int64
			if err := tx.Model(&models.SavedPaymentCard{}).Where("user_id = ?", input.UserID).Count(&count).Error; err != nil {
				return err
			}
			card.IsDefault = count == 0
		} else if err := tx.Model(&models.SavedPaymentCard{}).Where("user_id = ?", input.UserID).Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Create(&card).Error
	})
	if err != nil {
		return nil, err
	}
	return &card, nil
}

func (service *PaymentCardService) Delete(ctx context.Context, userID, cardID uint) error {
	result := service.database.WithContext(ctx).Where("id = ? AND user_id = ?", cardID, userID).Delete(&models.SavedPaymentCard{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrPaymentCardNotFound
	}
	return nil
}
