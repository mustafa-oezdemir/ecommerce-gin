package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
)

func viewData(c *gin.Context, data gin.H) gin.H {
	if data == nil {
		data = gin.H{}
	}
	data["CSRFField"] = csrf.TemplateField(c.Request)
	data["CSRFToken"] = csrf.Token(c.Request)
	return data
}
