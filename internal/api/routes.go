package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(router *gin.Engine, database *gorm.DB) {
	v1 := router.Group("/api/v1")
	products := v1.Group("/products")
	handler := NewProductHandler(database)
	products.GET("", handler.List)
	products.HEAD("", handler.List)
	products.OPTIONS("", handler.Options)
	products.GET("/:id", handler.Get)
	products.HEAD("/:id", handler.Get)
	products.OPTIONS("/:id", handler.Options)
}
