package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	sessionID = "123456"
	appDomain = "joe-calendar.example.net"
)

func TestFindCodeNotInitialized(t *testing.T) {
	found := FindCode("foo", appDomain)
	assert.Nil(t, found)
}

func TestBuildCode(t *testing.T) {
	code := BuildCode(sessionID, appDomain)
	assert.NotNil(t, code)
	assert.Equal(t, 22, len(code.Value))
	assert.Equal(t, sessionID, code.SessionID)
	assert.Equal(t, appDomain, code.AppHost)
}

func TestFindCode(t *testing.T) {
	sid := "987654"
	code := BuildCode(sid, appDomain)
	found := FindCode("foo", appDomain)
	assert.Nil(t, found)
	found = FindCode(code.Value, "joe-files.example.net")
	assert.Nil(t, found)
	found = FindCode(code.Value, appDomain)
	assert.NotNil(t, found)
	assert.Equal(t, sid, found.SessionID)
	// Code can be used only once
	found = FindCode(code.Value, appDomain)
	assert.Nil(t, found)
}

func TestExpiredCode(t *testing.T) {
	code := BuildCode("753159", appDomain)
	code.ExpiresAt -= 100
	found := FindCode(code.Value, appDomain)
	assert.Nil(t, found)
}
