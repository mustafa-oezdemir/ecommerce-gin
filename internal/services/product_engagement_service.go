package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const favoritesSystemKey = "favorites"

var (
	ErrReviewForbidden = errors.New("only verified purchasers can review this product")
	ErrReviewExists    = errors.New("a review already exists")
	ErrReviewNotFound  = errors.New("review not found")
	ErrInvalidReview   = errors.New("invalid review")
)

type ReviewSummary struct {
	Average      float64
	Count        int64
	Distribution [11]int64
}

type ReviewPage struct {
	Reviews    []models.ProductReview
	Summary    ReviewSummary
	Page       int
	PageSize   int
	TotalPages int
}

type ProductEngagementService struct{ database *gorm.DB }

func NewProductEngagementService(database *gorm.DB) *ProductEngagementService {
	if database == nil {
		panic("services: product engagement database is required")
	}
	return &ProductEngagementService{database: database}
}

func (s *ProductEngagementService) SetFavorite(ctx context.Context, userID, productID uint, enabled bool) error {
	if userID == 0 || productID == 0 {
		return ErrInvalidProductListInput
	}
	return s.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var product models.Product
		if err := tx.Where("id = ? AND active = ?", productID, true).First(&product).Error; err != nil {
			return ErrProductNotFound
		}
		var list models.ProductList
		err := tx.Where("user_id = ? AND system_key = ?", userID, favoritesSystemKey).First(&list).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			key := favoritesSystemKey
			list = models.ProductList{UserID: userID, Name: "Favorites", SystemKey: &key}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&list).Error; err != nil {
				return err
			}
			if list.ID == 0 && tx.Where("user_id = ? AND system_key = ?", userID, favoritesSystemKey).First(&list).Error != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if enabled {
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.ProductListItem{ProductListID: list.ID, ProductID: productID}).Error
		}
		return tx.Where("product_list_id = ? AND product_id = ?", list.ID, productID).Delete(&models.ProductListItem{}).Error
	})
}

func (s *ProductEngagementService) IsFavorite(ctx context.Context, userID, productID uint) (bool, error) {
	if userID == 0 {
		return false, nil
	}
	var count int64
	err := s.database.WithContext(ctx).Table("product_list_items pli").Joins("JOIN product_lists pl ON pl.id = pli.product_list_id").Where("pl.user_id = ? AND pl.system_key = ? AND pli.product_id = ?", userID, favoritesSystemKey, productID).Count(&count).Error
	return count > 0, err
}

func (s *ProductEngagementService) FavoriteProductIDs(ctx context.Context, userID uint) (map[uint]bool, error) {
	result := make(map[uint]bool)
	if userID == 0 {
		return result, nil
	}
	var ids []uint
	err := s.database.WithContext(ctx).Table("product_list_items pli").Select("pli.product_id").Joins("JOIN product_lists pl ON pl.id = pli.product_list_id").Where("pl.user_id = ? AND pl.system_key = ?", userID, favoritesSystemKey).Scan(&ids).Error
	for _, id := range ids {
		result[id] = true
	}
	return result, err
}

func (s *ProductEngagementService) CreateReview(ctx context.Context, userID, productID uint, rating uint8, title, body string) (*models.ProductReview, error) {
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if userID == 0 || productID == 0 || rating < 1 || rating > 10 || utf8.RuneCountInString(title) < 3 || utf8.RuneCountInString(title) > 150 || utf8.RuneCountInString(body) < 10 || utf8.RuneCountInString(body) > 5000 {
		return nil, ErrInvalidReview
	}
	verified, err := s.HasPurchased(ctx, userID, productID)
	if err != nil {
		return nil, err
	}
	if !verified {
		return nil, ErrReviewForbidden
	}
	review := &models.ProductReview{UserID: userID, ProductID: productID, Rating: rating, Title: title, Body: body}
	if err := s.database.WithContext(ctx).Create(review).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrReviewExists
		}
		return nil, fmt.Errorf("create review: %w", err)
	}
	return review, nil
}

func (s *ProductEngagementService) UpdateReview(ctx context.Context, userID, reviewID uint, rating uint8, title, body string) error {
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if userID == 0 || reviewID == 0 || rating < 1 || rating > 10 || utf8.RuneCountInString(title) < 3 || utf8.RuneCountInString(title) > 150 || utf8.RuneCountInString(body) < 10 || utf8.RuneCountInString(body) > 5000 {
		return ErrInvalidReview
	}
	result := s.database.WithContext(ctx).Model(&models.ProductReview{}).Where("id = ? AND user_id = ?", reviewID, userID).Updates(map[string]any{"rating": rating, "title": title, "body": body})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrReviewNotFound
	}
	return nil
}

func (s *ProductEngagementService) DeleteReview(ctx context.Context, userID, reviewID uint) (uint, error) {
	var review models.ProductReview
	if err := s.database.WithContext(ctx).Where("id = ? AND user_id = ?", reviewID, userID).First(&review).Error; err != nil {
		return 0, ErrReviewNotFound
	}
	if err := s.database.WithContext(ctx).Delete(&review).Error; err != nil {
		return 0, err
	}
	return review.ProductID, nil
}

func (s *ProductEngagementService) UserReview(ctx context.Context, userID, productID uint) (*models.ProductReview, error) {
	var review models.ProductReview
	if err := s.database.WithContext(ctx).Where("user_id = ? AND product_id = ?", userID, productID).First(&review).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &review, nil
}

func (s *ProductEngagementService) HasPurchased(ctx context.Context, userID, productID uint) (bool, error) {
	var count int64
	err := s.database.WithContext(ctx).Table("order_items oi").Joins("JOIN orders o ON o.id = oi.order_id").Where("o.user_id = ? AND oi.product_id = ? AND o.status IN ? AND o.deleted_at IS NULL AND oi.deleted_at IS NULL", userID, productID, []models.OrderStatus{models.OrderStatusShipped, models.OrderStatusCompleted}).Count(&count).Error
	return count > 0, err
}

func (s *ProductEngagementService) Reviews(ctx context.Context, productID uint, page int, sort string) (*ReviewPage, error) {
	if page < 1 {
		page = 1
	}
	const pageSize = 10
	order := "product_reviews.created_at DESC"
	switch sort {
	case "oldest":
		order = "product_reviews.created_at ASC"
	case "highest":
		order = "rating DESC, product_reviews.created_at DESC"
	case "lowest":
		order = "rating ASC, product_reviews.created_at DESC"
	}
	summary, err := s.ReviewSummary(ctx, productID)
	if err != nil {
		return nil, err
	}
	var reviews []models.ProductReview
	if err := s.database.WithContext(ctx).Preload("User").Where("product_id = ?", productID).Order(order).Limit(pageSize).Offset((page - 1) * pageSize).Find(&reviews).Error; err != nil {
		return nil, err
	}
	totalPages := int((summary.Count + pageSize - 1) / pageSize)
	return &ReviewPage{Reviews: reviews, Summary: summary, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func (s *ProductEngagementService) ReviewSummary(ctx context.Context, productID uint) (ReviewSummary, error) {
	var rows []struct {
		Rating uint8
		Count  int64
	}
	if err := s.database.WithContext(ctx).Model(&models.ProductReview{}).Select("rating, COUNT(*) AS count").Where("product_id = ?", productID).Group("rating").Scan(&rows).Error; err != nil {
		return ReviewSummary{}, err
	}
	var summary ReviewSummary
	var total int64
	for _, row := range rows {
		summary.Distribution[row.Rating] = row.Count
		summary.Count += row.Count
		total += int64(row.Rating) * row.Count
	}
	if summary.Count > 0 {
		summary.Average = float64(total) / float64(summary.Count)
	}
	return summary, nil
}

func (s *ProductEngagementService) ReviewSummaries(ctx context.Context, productIDs []uint) (map[uint]ReviewSummary, error) {
	result := make(map[uint]ReviewSummary)
	if len(productIDs) == 0 {
		return result, nil
	}
	var rows []struct {
		ProductID uint
		Rating    uint8
		Count     int64
	}
	if err := s.database.WithContext(ctx).Model(&models.ProductReview{}).Select("product_id, rating, COUNT(*) AS count").Where("product_id IN ?", productIDs).Group("product_id, rating").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		item := result[row.ProductID]
		item.Distribution[row.Rating] = row.Count
		item.Count += row.Count
		item.Average += float64(row.Rating) * float64(row.Count)
		result[row.ProductID] = item
	}
	for id, item := range result {
		if item.Count > 0 {
			item.Average /= float64(item.Count)
		}
		result[id] = item
	}
	return result, nil
}
