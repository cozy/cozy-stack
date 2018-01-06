package vfs

import (
	"encoding/hex"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/utils"
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

func TestGenerateSecureLinkSecret(t *testing.T) {
	var keys [][]byte
	var docs []*FileDoc
	var sessionIDs []string

	n := 5

	for i := 0; i < n; i++ {
		keys = append(keys, crypto.GenerateRandomBytes(32))
	}
	for i := 0; i < n; i++ {
		docs = append(docs, &FileDoc{DocID: utils.RandomString(10), DocRev: utils.RandomString(10)})
	}
	for i := 0; i < n-1; i++ {
		sessionIDs = append(sessionIDs, utils.RandomString(10))
	}
	sessionIDs = append(sessionIDs, "") // test an the valid usecase of empty sessionID (no session)

	secrets := make([]string, 0, n*n*n)
	for x := 0; x < n; x++ {
		for y := 0; y < n; y++ {
			for z := 0; z < n; z++ {
				secrets = append(secrets, GenerateSecureLinkSecret(keys[x], docs[y], sessionIDs[z]))
			}
		}
	}

	for i, secret := range secrets {
		for x := 0; x < n; x++ {
			for y := 0; y < n; y++ {
				for z := 0; z < n; z++ {
					ok := VerifySecureLinkSecret(keys[x], secret, docs[y].DocID, sessionIDs[z])
					if i == x*n*n+y*n+z {
						assert.True(t, ok)
					} else {
						assert.False(t, ok)
					}
				}
			}
		}
	}
}

var result interface{}

func BenchmarkGenerateLinkSecret(b *testing.B) {
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
