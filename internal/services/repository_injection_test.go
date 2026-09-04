package services

import (
	"context"
	"errors"
	"testing"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

type cartRepositoryStub struct {
	getOrCreate func(context.Context, uint) (*models.Cart, error)
}

func (s cartRepositoryStub) GetOrCreateCart(ctx context.Context, userID uint) (*models.Cart, error) {
	return s.getOrCreate(ctx, userID)
}
func (cartRepositoryStub) AddItem(context.Context, uint, uint, int) error { return nil }
func (cartRepositoryStub) ClearCart(context.Context, uint) error          { return nil }
func (cartRepositoryStub) UpdateQuantityForUser(context.Context, uint, uint, int) (bool, error) {
	return false, nil
}
func (cartRepositoryStub) RemoveItemForUser(context.Context, uint, uint) (bool, error) {
	return false, nil
}

type orderRepositoryStub struct {
	list func(context.Context, uint) ([]models.Order, error)
}

func (s orderRepositoryStub) ListByUserID(ctx context.Context, userID uint) ([]models.Order, error) {
	return s.list(ctx, userID)
}
func (orderRepositoryStub) GetByIDForUser(context.Context, uint, uint) (*models.Order, error) {
	return nil, nil
}

type productListRepositoryStub struct {
	create func(context.Context, *models.ProductList) error
}

func (productListRepositoryStub) ListByUserID(context.Context, uint) ([]models.ProductList, error) {
	return nil, nil
}
func (productListRepositoryStub) GetByIDForUser(context.Context, uint, uint) (*models.ProductList, error) {
	return nil, nil
}
func (s productListRepositoryStub) Create(ctx context.Context, list *models.ProductList) error {
	return s.create(ctx, list)
}
func (productListRepositoryStub) AddProduct(context.Context, uint, uint) error { return nil }
func (productListRepositoryStub) RemoveProduct(context.Context, uint, uint, uint) (bool, error) {
	return false, nil
}
func (productListRepositoryStub) Delete(context.Context, uint, uint) (bool, error) {
	return false, nil
}

func TestCartServiceUsesInjectedRepository(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "cart-test")
	want := &models.Cart{UserID: 42}
	service := newCartService(nil, cartRepositoryStub{getOrCreate: func(gotCtx context.Context, userID uint) (*models.Cart, error) {
		if gotCtx.Value(contextKey("request")) != "cart-test" {
			t.Fatal("request context was not propagated")
		}
		if userID != 42 {
			t.Fatalf("expected user ID 42, got %d", userID)
		}
		return want, nil
	}})

	got, err := service.GetCart(ctx, models.User{Model: gorm.Model{ID: 42}})
	if err != nil {
		t.Fatalf("get cart: %v", err)
	}
	if got != want {
		t.Fatal("service did not return the repository result")
	}
}

func TestOrderServiceUsesInjectedRepository(t *testing.T) {
	wantErr := errors.New("repository unavailable")
	service := newOrderService(nil, orderRepositoryStub{list: func(_ context.Context, userID uint) ([]models.Order, error) {
		if userID != 7 {
			t.Fatalf("expected user ID 7, got %d", userID)
		}
		return nil, wantErr
	}})

	_, err := service.ListUserOrders(context.Background(), 7)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
}

func TestProductListServiceNormalizesInputBeforeRepository(t *testing.T) {
	service := newProductListService(nil, productListRepositoryStub{create: func(_ context.Context, list *models.ProductList) error {
		if list.UserID != 9 || list.Name != "Weekly shopping" {
			t.Fatalf("unexpected product list: %#v", list)
		}
		return nil
	}})

	if err := service.Create(context.Background(), 9, "  Weekly shopping  "); err != nil {
		t.Fatalf("create product list: %v", err)
	}
}
