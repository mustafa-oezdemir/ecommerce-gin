package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
)

type ProductEngagementHandler struct {
	engagement *services.ProductEngagementService
	lists      *services.ProductListService
}

func NewProductEngagementHandler(engagement *services.ProductEngagementService, lists *services.ProductListService) *ProductEngagementHandler {
	return &ProductEngagementHandler{engagement: engagement, lists: lists}
}

func (h *ProductEngagementHandler) AddFavorite(c *gin.Context)    { h.setFavorite(c, true) }
func (h *ProductEngagementHandler) RemoveFavorite(c *gin.Context) { h.setFavorite(c, false) }

func (h *ProductEngagementHandler) setFavorite(c *gin.Context, enabled bool) {
	user, ok := middleware.CurrentUser(c)
	productID, parseOK := positiveID(c.Param("id"))
	if !ok || !parseOK {
		jsonError(c, http.StatusBadRequest, "invalid_request", "Invalid product")
		return
	}
	if err := h.engagement.SetFavorite(c.Request.Context(), user.ID, productID, enabled); err != nil {
		jsonError(c, http.StatusNotFound, "product_not_found", "Product not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"product_id": productID, "favorite": enabled}})
}

func (h *ProductEngagementHandler) AddToList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	productID, productOK := positiveID(c.Param("id"))
	listID64, listErr := strconv.ParseUint(c.PostForm("list_id"), 10, 64)
	if !ok || !productOK || listErr != nil || listID64 == 0 {
		jsonError(c, http.StatusBadRequest, "invalid_request", "Invalid list or product")
		return
	}
	if err := h.lists.AddProduct(c.Request.Context(), user.ID, uint(listID64), productID); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrProductListNotFound) || errors.Is(err, services.ErrProductNotFound) {
			status = http.StatusNotFound
		}
		jsonError(c, status, "list_update_failed", "Could not add product to list")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"product_id": productID, "list_id": uint(listID64)}})
}

func (h *ProductEngagementHandler) CreateReview(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	productID, productOK := positiveID(c.Param("id"))
	rating, ratingErr := strconv.ParseUint(c.PostForm("rating"), 10, 8)
	if !ok || !productOK || ratingErr != nil {
		jsonError(c, http.StatusBadRequest, "invalid_review", "Invalid review")
		return
	}
	review, err := h.engagement.CreateReview(c.Request.Context(), user.ID, productID, uint8(rating), c.PostForm("title"), c.PostForm("body"))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrReviewForbidden):
			jsonError(c, http.StatusForbidden, "verified_purchase_required", "Only verified purchasers can review this product")
		case errors.Is(err, services.ErrReviewExists):
			jsonError(c, http.StatusConflict, "review_exists", "You have already reviewed this product")
		default:
			jsonError(c, http.StatusBadRequest, "invalid_review", "The review could not be saved")
		}
		return
	}
	summary, _ := h.engagement.ReviewSummary(c.Request.Context(), productID)
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": gin.H{"id": review.ID, "rating": review.Rating, "title": review.Title, "body": review.Body, "author": user.Name, "verified_purchase": true, "summary": summary}})
}

func (h *ProductEngagementHandler) UpdateReview(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	reviewID, idOK := positiveID(c.Param("id"))
	rating, ratingErr := strconv.ParseUint(c.PostForm("rating"), 10, 8)
	if !ok || !idOK || ratingErr != nil {
		jsonError(c, http.StatusBadRequest, "invalid_review", "Invalid review")
		return
	}
	if err := h.engagement.UpdateReview(c.Request.Context(), user.ID, reviewID, uint8(rating), c.PostForm("title"), c.PostForm("body")); err != nil {
		if errors.Is(err, services.ErrReviewNotFound) {
			jsonError(c, http.StatusNotFound, "review_not_found", "Review not found")
			return
		}
		jsonError(c, http.StatusBadRequest, "invalid_review", "The review could not be saved")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": reviewID, "rating": uint8(rating), "title": c.PostForm("title"), "body": c.PostForm("body")}})
}

func (h *ProductEngagementHandler) DeleteReview(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	reviewID, idOK := positiveID(c.Param("id"))
	if !ok || !idOK {
		jsonError(c, http.StatusBadRequest, "invalid_request", "Invalid review")
		return
	}
	productID, err := h.engagement.DeleteReview(c.Request.Context(), user.ID, reviewID)
	if err != nil {
		jsonError(c, http.StatusNotFound, "review_not_found", "Review not found")
		return
	}
	summary, _ := h.engagement.ReviewSummary(c.Request.Context(), productID)
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": gin.H{"id": reviewID, "deleted": true, "summary": summary}})
}

func positiveID(value string) (uint, bool) {
	id, err := strconv.ParseUint(value, 10, 64)
	return uint(id), err == nil && id > 0
}

func jsonError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"ok": false, "error": gin.H{"code": code, "message": message}})
}
