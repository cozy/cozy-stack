package permissions

import (
	"net/http"

	"github.com/labstack/echo"
)

var (
	// ErrInvalidToken is used when the token is invalid (the signature is not
	// correct, the domain is not the good one, etc.)
	ErrInvalidToken = echo.NewHTTPError(http.StatusBadRequest,
		"Invalid JWT token")

	// ErrInvalidAudience is used when the audience is not expected
	ErrInvalidAudience = echo.NewHTTPError(http.StatusBadRequest,
		"Invalid audience for JWT token")

	// ErrExpiredToken is used when the token has expired and the client should
	// refresh it
	ErrExpiredToken = echo.NewHTTPError(http.StatusBadRequest,
		"Expired token")

	// ErrBadScope is used when the given scope is malformed
	ErrBadScope = echo.NewHTTPError(http.StatusBadRequest,
		"Permission scope is empty or malformed")
)
