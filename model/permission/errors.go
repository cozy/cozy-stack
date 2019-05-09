package permission

import (
	"net/http"

	"github.com/cozy/echo"
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

	// ErrNotSubset is returned on requests attempting to create a Set of
	// permissions which is not a subset of the request's own token.
	ErrNotSubset = echo.NewHTTPError(http.StatusForbidden,
		"Attempt to create a larger permission set")

	// ErrOnlyAppCanCreateSubSet is returned if a non-app attempts to create
	// sharing permissions.
	ErrOnlyAppCanCreateSubSet = echo.NewHTTPError(http.StatusForbidden,
		"Only apps can create sharing permissions")

	// ErrNotParent is used when the permissions should have a specific parent.
	ErrNotParent = echo.NewHTTPError(http.StatusForbidden,
		"Permissions can be updated only by its parent")
)
