package vfs

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/stretchr/testify/assert"
)

func TestDownloadStoreInMemory(t *testing.T) {
	downloadStoreTTL = 100 * time.Millisecond

	domainA := "alice.cozycloud.local"
	store := newMemStore()

	a := &Archive{
		Name: "test",
		Files: []string{
			"/archive/foo.jpg",
			"/archive/bar",
		},
	}
	key2, err := store.AddArchive(domainA, a)
	assert.NoError(t, err)

	a2, err := store.GetArchive(domainA, key2)
	assert.NoError(t, err)
	assert.Equal(t, a, a2)

	time.Sleep(2 * downloadStoreTTL)

	a3, err := store.GetArchive(domainA, key2)
	assert.NoError(t, err)
	assert.Nil(t, a3, "no expiration")
}

func TestDownloadStoreInRedis(t *testing.T) {
	downloadStoreTTL = 100 * time.Millisecond

	domainA := "alice.cozycloud.local"
	store := GetStore()

	a := &Archive{
		Name: "test",
		Files: []string{
			"/archive/foo.jpg",
			"/archive/bar",
		},
	}
	key2, err := store.AddArchive(domainA, a)
	assert.NoError(t, err)

	a2, err := store.GetArchive(domainA, key2)
	assert.NoError(t, err)
	assert.Equal(t, a, a2)

	time.Sleep(2 * downloadStoreTTL)

	a3, err := store.GetArchive(domainA, key2)
	assert.NoError(t, err)
	assert.Nil(t, a3, "no expiration")
}

var result interface{}

func BenchmarkGenerateLinkSecret(b *testing.B) {
	fmt.Println(">>>>>", time.Now(), time.Now().UTC())
	fmt.Println(">>>>>", time.Now().Unix(), time.Now().UTC().Unix())
	key := crypto.GenerateRandomBytes(64)
	ids := make([]string, 256)
	for i := range ids {
		ids[i] = hex.EncodeToString(crypto.GenerateRandomBytes(32))
	}

	secrets := make([]string, 50)
	for i := 0; i < b.N; i++ {
		for n := 0; n < 50; n++ {
			s := GenerateSecureLinkSecret(key, &FileDoc{DocID: ids[i%256], DocRev: ids[(i+1)%256]}, ids[(i+2)%256])
			secrets[n] = s
		}
	}

	result = secrets
}

func BenchmarkSerializeFiles(b *testing.B) {
	files := make([]*FileDoc, 50)
	for i := range files {
		files[i] = &FileDoc{
			Type:        "file",
			DocID:       hex.EncodeToString(crypto.GenerateRandomBytes(16)),
			DocRev:      hex.EncodeToString(crypto.GenerateRandomBytes(16)),
			DocName:     "MyNameIs" + strconv.Itoa(i),
			DirID:       hex.EncodeToString(crypto.GenerateRandomBytes(16)),
			RestorePath: "",
			ByteSize:    6341,
			MD5Sum:      crypto.GenerateRandomBytes(16),
			Mime:        "application/octet-stream",
			Class:       "image",
			Executable:  false,
			Trashed:     false,
			Tags:        []string{"foo", "bar"},
		}
	}

	var r []byte
	for i := 0; i < b.N; i++ {
		var err error
		r, err = json.Marshal(files)
		if err != nil {
			b.Fatal(err)
		}
	}

	result = r
}
