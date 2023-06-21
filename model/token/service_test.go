package token

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/cache"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/stretchr/testify/assert"
)

func TestServiceImplem(t *testing.T) {
	assert.Implements(t, (*Service)(nil), new(TokenService))
	assert.Implements(t, (*Service)(nil), new(Mock))
}

func Test_CacheService_success(t *testing.T) {
	cache := cache.NewInMemory()

	db := prefixer.NewPrefixer(1, "foo", "global")

	svc := NewService(cache)

	token, err := svc.GenerateAndSave(db, MagicLink, "some-url", time.Second)
	assert.NoError(t, err)
	assert.Len(t, token, TokenLen)

	err = svc.Validate(db, MagicLink, "some-url", token)
	assert.NoError(t, err)
}

func Test_CacheService_with_an_invalid_namespace(t *testing.T) {
	cache := cache.NewInMemory()

	db := prefixer.NewPrefixer(1, "foo", "global")

	svc := NewService(cache)

	token, err := svc.GenerateAndSave(db, "invalid", "foo", time.Millisecond)
	assert.Empty(t, token)
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func Test_CacheService_expired_token(t *testing.T) {
	cache := cache.NewInMemory()

	db := prefixer.NewPrefixer(1, "foo", "global")

	svc := NewService(cache)

	token, err := svc.GenerateAndSave(db, EmailUpdate, "foo@bar.baz", time.Millisecond)
	assert.NoError(t, err)
	assert.Len(t, token, TokenLen)

	time.Sleep(5 * time.Millisecond)

	err = svc.Validate(db, EmailUpdate, "foo@bar.baz", token)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func Test_CacheService_scope_by_resource(t *testing.T) {
	cache := cache.NewInMemory()

	db := prefixer.NewPrefixer(1, "foo", "global")

	svc := NewService(cache)

	token, err := svc.GenerateAndSave(db, EmailUpdate, "foo@bar.baz", time.Second)
	assert.NoError(t, err)
	assert.Len(t, token, TokenLen)

	// Failed because the email is not the same.
	err = svc.Validate(db, EmailUpdate, "another@email.com", token)
	assert.ErrorIs(t, err, ErrInvalidToken)

	err = svc.Validate(db, EmailUpdate, "foo@bar.baz", token)
	assert.NoError(t, err)
}
