package utils

import "github.com/gin-gonic/gin"

// APIError is the shape of every error response from the API.
// { "error": { "code": "...", "message": "...", "details": ... } }
type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details"`
}

// ErrorResponse writes a structured error response.
// Use this everywhere instead of c.JSON with a raw map.
func ErrorResponse(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": APIError{
			Code:    code,
			Message: message,
			Details: nil,
		},
	})
}

// ValidationErrorResponse writes a 400 with field-level detail.
func ValidationErrorResponse(c *gin.Context, field, message string) {
	c.AbortWithStatusJSON(400, gin.H{
		"error": APIError{
			Code:    "VALIDATION_ERROR",
			Message: "input validation failed",
			Details: gin.H{
				"field":   field,
				"message": message,
			},
		},
	})
}

// common error shortcuts used across handlers and middleware
func ErrUnauthorized(c *gin.Context, code, message string) {
	ErrorResponse(c, 401, code, message)
}

func ErrForbidden(c *gin.Context, message string) {
	ErrorResponse(c, 403, "INSUFFICIENT_PERMISSIONS", message)
}

func ErrNotFound(c *gin.Context, message string) {
	ErrorResponse(c, 404, "NOT_FOUND", message)
}

func ErrConflict(c *gin.Context, code, message string) {
	ErrorResponse(c, 409, code, message)
}

func ErrInternal(c *gin.Context) {
	ErrorResponse(c, 500, "INTERNAL_ERROR", "something went wrong")
}