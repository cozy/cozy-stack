package logger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger_MemDebugger_Success(t *testing.T) {
	dbg := NewMemDebugger()

	_ = dbg.AddDomain("foo", time.Second)

	expirationDate := dbg.ExpiresAt("foo")
	assert.NotNil(t, expirationDate)
}

func TestLogger_MemDebugger_Delete(t *testing.T) {
	dbg := NewMemDebugger()

	_ = dbg.AddDomain("foo", time.Second)

	_ = dbg.RemoveDomain("foo")

	expirationDate := dbg.ExpiresAt("foo")
	assert.Nil(t, expirationDate)
}

func TestLogger_MemDebugger_Expire(t *testing.T) {
	dbg := NewMemDebugger()

	_ = dbg.AddDomain("foo", 2*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	expirationDate := dbg.ExpiresAt("foo")
	assert.Nil(t, expirationDate)
}

func TestLogger_MemDebugger_Override_and_expire(t *testing.T) {
	dbg := NewMemDebugger()

	// First setup a domain with a 1s TTL
	dbg.AddDomain("foo", time.Second)

	// Check that we have 1s
	expirationDate := dbg.ExpiresAt("foo")
	require.NotNil(t, expirationDate)
	require.WithinDuration(t, time.Now().Add(time.Second), *expirationDate, 5*time.Millisecond)

	// Then move the domain to 2ms TTL
	_ = dbg.AddDomain("foo", 2*time.Millisecond)

	// Check that we have 2ms
	expirationDate = dbg.ExpiresAt("foo")
	require.NotNil(t, expirationDate)
	require.WithinDuration(t, time.Now().Add(2*time.Millisecond), *expirationDate, 2*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	// Check the correct expiration
	expirationDate = dbg.ExpiresAt("foo")
	assert.Nil(t, expirationDate)
}
