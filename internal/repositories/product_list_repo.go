package repositories

import (
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

type ProductListRepository struct{}

func NewProductListRepository() *ProductListRepository { return &ProductListRepository{} }

func (r *ProductListRepository) ListByUserID(userID uint) ([]models.ProductList, error) {
	var lists []models.ProductList
	err := db.DB.Preload("Items.Product.Category").Where("user_id = ?", userID).Order("created_at DESC").Find(&lists).Error
	return lists, err
}

func (r *ProductListRepository) GetByIDForUser(listID, userID uint) (*models.ProductList, error) {
	var list models.ProductList
	if err := db.DB.Preload("Items.Product.Category").Where("id = ? AND user_id = ?", listID, userID).First(&list).Error; err != nil {
		return nil, err
	}
	return &list, nil
}

func (r *ProductListRepository) Create(list *models.ProductList) error {
	return db.DB.Create(list).Error
}

func (r *ProductListRepository) AddProduct(listID, productID uint) error {
	var count int64
	if err := db.DB.Model(&models.ProductListItem{}).Where("product_list_id = ? AND product_id = ?", listID, productID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.DB.Create(&models.ProductListItem{ProductListID: listID, ProductID: productID}).Error
}

func (r *ProductListRepository) RemoveProduct(listID, userID, productID uint) (bool, error) {
	result := db.DB.Where("product_list_id = ? AND product_id = ? AND EXISTS (SELECT 1 FROM product_lists WHERE product_lists.id = ? AND product_lists.user_id = ?)", listID, productID, listID, userID).Delete(&models.ProductListItem{})
	return result.RowsAffected == 1, result.Error
}

func (r *ProductListRepository) Delete(listID, userID uint) (bool, error) {
	result := db.DB.Where("id = ? AND user_id = ?", listID, userID).Delete(&models.ProductList{})
	return result.RowsAffected == 1, result.Error
}
