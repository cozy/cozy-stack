package vfs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDonwloadStore(t *testing.T) {
	domainA := "alice.cozycloud.local"
	domainB := "bob.cozycloud.local"
	storeA := GetStore(domainA)
	storeB := GetStore(domainB)

	path := "/test/random/path.txt"
	key1, err := storeA.AddFile(path)
	assert.NoError(t, err)

	path2, err := storeB.GetFile(key1)
	assert.NoError(t, err)
	assert.Zero(t, path2, "Inter-instances store leaking")

	path3, err := storeA.GetFile(key1)
	assert.NoError(t, err)
	assert.Equal(t, path, path3)

	storeStore[domainA].Files[key1].ExpiresAt = time.Now().Add(-2 * downloadStoreTTL)

	path4, err := storeA.GetFile(key1)
	assert.NoError(t, err)
	assert.Zero(t, path4, "no expiration")

	a := &Archive{
		Name: "test",
		Files: []string{
			"/archive/foo.jpg",
			"/archive/bar",
		},
	}
	key2, err := storeA.AddArchive(a)
	assert.NoError(t, err)

	a2, err := storeA.GetArchive(key2)
	assert.NoError(t, err)
	assert.Equal(t, a, a2)

	storeStore[domainA].Archives[key2].ExpiresAt = time.Now().Add(-2 * downloadStoreTTL)

	a3, err := storeA.GetArchive(key2)
	assert.NoError(t, err)
	assert.Nil(t, a3, "no expiration")
}
