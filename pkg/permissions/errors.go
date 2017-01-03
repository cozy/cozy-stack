package permissions

import "errors"

var (
	// ErrInvalidToken is used when the token is invalid (the signature is not
	// correct, the domain is not the good one, etc.)
	ErrInvalidToken = errors.New("Invalid JWT token")

	// ErrInvalidAudience is used when the audience is not expected
	ErrInvalidAudience = errors.New("Invalid audience for JWT token")
)
