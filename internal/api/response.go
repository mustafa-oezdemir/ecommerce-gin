package api

import "github.com/gin-gonic/gin"

type response struct {
	Success bool        `json:"success"`
	Data    any         `json:"data,omitempty"`
	Error   *errorInfo  `json:"error,omitempty"`
	Meta    *pagination `json:"meta,omitempty"`
}

type errorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type pagination struct {
	Limit      int   `json:"limit"`
	Offset     int   `json:"offset"`
	Page       int   `json:"page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

func respondOK(c *gin.Context, status int, data any, meta *pagination) {
	c.JSON(status, response{Success: true, Data: data, Meta: meta})
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, response{Success: false, Error: &errorInfo{Code: code, Message: message}})
}
