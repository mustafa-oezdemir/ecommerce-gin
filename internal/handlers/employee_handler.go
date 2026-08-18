package handlers

import (
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
    "github.com/mustafa/ecommerce-gin/internal/db"
    "github.com/mustafa/ecommerce-gin/internal/models"
)

type EmployeeHandler struct{}

func NewEmployeeHandler() *EmployeeHandler {
    return &EmployeeHandler{}
}

func (h *EmployeeHandler) ListProducts(c *gin.Context) {
    var products []models.Product
    db.DB.Find(&products)
    c.HTML(http.StatusOK, "employee_products.tmpl", gin.H{
        "Products": products,
    })
}

func (h *EmployeeHandler) CreateProduct(c *gin.Context) {
    name := c.PostForm("name")
    desc := c.PostForm("description")
    priceStr := c.PostForm("price")
    stockStr := c.PostForm("stock")

    price, _ := strconv.ParseFloat(priceStr, 64)
    stock, _ := strconv.Atoi(stockStr)

    p := models.Product{
        Name:        name,
        Description: desc,
        Price:       price,
        Stock:       stock,
    }

    if err := db.DB.Create(&p).Error; err != nil {
        c.String(http.StatusInternalServerError, "Error creating product")
        return
    }

    c.Redirect(http.StatusFound, "/employee/products")
}

func (h *EmployeeHandler) UpdateStock(c *gin.Context) {
    id := c.Param("id")
    stockStr := c.PostForm("stock")
    stock, _ := strconv.Atoi(stockStr)

    var p models.Product
    if err := db.DB.First(&p, id).Error; err != nil {
        c.String(http.StatusNotFound, "Product not found")
        return
    }

    p.Stock = stock
    db.DB.Save(&p)

    c.Redirect(http.StatusFound, "/employee/products")
}
