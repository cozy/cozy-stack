package token

import (
	"errors"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/cache"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

const TokenLen = 32

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrInvalidNamespace = errors.New("invalid namespace")
)

type Operation string

var (
	EmailUpdate Operation = "email_update"
	MagicLink   Operation = "magic_link"
)

// TokenService is a [Service] implementation based on [cache.Cache].
//
// Note: Depending on the cache implementation setup the storage can
// be store in-memory a reboot would invalidate all the existing tokens.
//
// This can be the case for the self-hosted stacks using [cache.InMemory].
type TokenService struct {
	cache cache.Cache
}

// NewService instantiates a new [CacheService].
func NewService(cache cache.Cache) *TokenService {
	return &TokenService{cache}
}

// GenerateAndSave generate a random token and save it into the storage for the specified duration.
//
// Once the lifetime is expired, the token is deleted and will be never valid.
func (s *TokenService) GenerateAndSave(db prefixer.Prefixer, op Operation, resource string, lifetime time.Duration) (string, error) {
	token := crypto.GenerateRandomString(TokenLen)

	key, err := generateKey(db, op, token)
	if err != nil {
		return "", err
	}

	// TODO: The cache should returns an error and should be matched here.
	s.cache.SetNX(key, []byte(resource), lifetime)

	return token, nil
}

// Validate will validate that the user as a matching token.
//
// If the token doesn't match any content or an expired content the error [ErrInvalidToken] is returned.
func (s *TokenService) Validate(db prefixer.Prefixer, op Operation, resource, token string) error {
	key, err := generateKey(db, op, token)
	if err != nil {
		return err
	}

	// TODO: The cache should return an error and we should parse it.
	val, ok := s.cache.Get(key)
	if !ok {
		return ErrInvalidToken
	}

	if string(val) != resource {
		return ErrInvalidToken
	}

	// TODO: Should we delete automatically the token valid once validated?

	return nil
}

func generateKey(db prefixer.Prefixer, ns Operation, code string) (string, error) {
	var nsStr string

	switch ns {
	case EmailUpdate, MagicLink:
		nsStr = string(ns)
	default:
		return "", ErrInvalidNamespace
	}

	return strings.Join([]string{db.DBPrefix(), nsStr, code}, ":"), nil
}
