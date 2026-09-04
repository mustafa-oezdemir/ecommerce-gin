package repositories

import (
	"context"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProductListRepository struct{ database *gorm.DB }

func NewProductListRepository(database *gorm.DB) *ProductListRepository {
	return &ProductListRepository{database: database}
}

func (r *ProductListRepository) ListByUserID(ctx context.Context, userID uint) ([]models.ProductList, error) {
	var lists []models.ProductList
	err := r.database.WithContext(ctx).Preload("Items.Product.Category").Where("user_id = ?", userID).Order("created_at DESC").Find(&lists).Error
	return lists, err
}

func (r *ProductListRepository) GetByIDForUser(ctx context.Context, listID, userID uint) (*models.ProductList, error) {
	var list models.ProductList
	if err := r.database.WithContext(ctx).Preload("Items.Product.Category").Where("id = ? AND user_id = ?", listID, userID).First(&list).Error; err != nil {
		return nil, err
	}
	return &list, nil
}

func (r *ProductListRepository) Create(ctx context.Context, list *models.ProductList) error {
	return r.database.WithContext(ctx).Create(list).Error
}

func (r *ProductListRepository) AddProduct(ctx context.Context, listID, productID uint) error {
	return r.database.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.ProductListItem{ProductListID: listID, ProductID: productID}).Error
}

func (r *ProductListRepository) RemoveProduct(ctx context.Context, listID, userID, productID uint) (bool, error) {
	result := r.database.WithContext(ctx).Where("product_list_id = ? AND product_id = ? AND EXISTS (SELECT 1 FROM product_lists WHERE product_lists.id = ? AND product_lists.user_id = ?)", listID, productID, listID, userID).Delete(&models.ProductListItem{})
	return result.RowsAffected == 1, result.Error
}

func (r *ProductListRepository) Delete(ctx context.Context, listID, userID uint) (bool, error) {
	result := r.database.WithContext(ctx).Where("id = ? AND user_id = ?", listID, userID).Delete(&models.ProductList{})
	return result.RowsAffected == 1, result.Error
}
