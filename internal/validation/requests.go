package validation

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

type LoginRequest struct {
	Email    string `form:"email" binding:"required,email,max=254"`
	Password string `form:"password" binding:"required,max=72"`
}

type CreateUserRequest struct {
	Name     string `form:"name" binding:"required,min=2,max=100"`
	Email    string `form:"email" binding:"required,email,max=254"`
	Password string `form:"password" binding:"required,min=12,max=72"`
	Role     string `form:"role" binding:"required,oneof=employee customer"`
}

type UpdateProfileRequest struct {
	Name  string `form:"name" binding:"required,min=2,max=100"`
	Email string `form:"email" binding:"required,email,max=254"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `form:"current_password" binding:"required,max=72"`
	NewPassword     string `form:"new_password" binding:"required,min=12,max=72"`
}

type CreateProductRequest struct {
	Name        string `form:"name" binding:"required,min=2,max=200"`
	Description string `form:"description" binding:"max=5000"`
	Price       string `form:"price" binding:"required,max=32"`
	Stock       int    `form:"stock" binding:"gte=0,lte=1000000"`
	CategoryID  uint   `form:"category_id" binding:"omitempty,gt=0"`
}

type UpdateStockRequest struct {
	Stock int `form:"stock" binding:"gte=0,lte=1000000"`
}

type ProductIDURI struct {
	ID uint `uri:"id" binding:"required,gt=0"`
}

type CartItemIDURI struct {
	ID uint `uri:"id" binding:"required,gt=0"`
}

type AddToCartRequest struct {
	Quantity int `form:"quantity,default=1" binding:"gte=1,lte=100"`
}

type UpdateQuantityRequest struct {
	Quantity int `form:"quantity" binding:"gte=1,lte=100"`
}

type UpdateOrderStatusRequest struct {
	Status string `form:"status" binding:"required,oneof=processing shipped completed cancelled"`
}

type CreateCategoryRequest struct {
	Name        string `form:"name" binding:"required,min=2,max=100"`
	Description string `form:"description" binding:"max=5000"`
}

var pricePattern = regexp.MustCompile(`^[0-9]+(?:[.,][0-9]{1,2})?$`)

func ParseCents(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if !pricePattern.MatchString(value) {
		return 0, errors.New("invalid price")
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '.' || r == ',' })
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 || whole > 1000000 {
		return 0, errors.New("invalid price")
	}
	cents := int64(0)
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
		cents, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, errors.New("invalid price")
		}
	}
	result := whole*100 + cents
	if result <= 0 {
		return 0, errors.New("invalid price")
	}
	return result, nil
}
