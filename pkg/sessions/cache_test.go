package sessions

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/stretchr/testify/assert"
)

var testDB = prefixer.NewPrefixer("cached.cozy.tools", "cached.cozy.tools")

func TestCache(t *testing.T) {
	globalCache = nil
	defer func() {
		globalCache = nil
	}()

	s := &Session{
		DocID:    "2caafe00-8c05-11e7-bea5-0792f2ba5a60",
		LastSeen: time.Now(),
	}
	getCache().Set(testDB, s.DocID, s)

	s2 := getCache().Get(testDB, s.DocID)
	assert.NotNil(t, s2)
	assert.Equal(t, s.DocID, s2.DocID)
	assert.Equal(t, s.LastSeen.Unix(), s2.LastSeen.Unix())

	globalCache.Revoke(testDB, s.DocID)

	s3 := getCache().Get(testDB, s.DocID)
	assert.Nil(t, s3)

}
