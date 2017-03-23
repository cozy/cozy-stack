package vfs

import (
	"encoding/hex"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/crypto"
)

// A DownloadStore is essentially an object to store Archives & Files by keys
type DownloadStore interface {
	AddFile(f string) (string, error)
	AddArchive(a *Archive) (string, error)
	GetFile(k string) (string, error)
	GetArchive(k string) (*Archive, error)
}

type fileRef struct {
	Path      string
	ExpiresAt time.Time
}

// downloadStoreTTL is the time an Archive stay alive
const downloadStoreTTL = 1 * time.Hour

// downloadStoreCleanInterval is the time interval between each download
// cleanup.
const downloadStoreCleanInterval = 1 * time.Hour

var storeStoreMutex sync.Mutex
var storeStore map[string]*memStore

func init() {
	go cleanDownloadStoreInterval()
}

func cleanDownloadStoreInterval() {
	for range time.Tick(downloadStoreCleanInterval) {
		cleanDownloadStore()
	}
}

func cleanStore(s *memStore) {
	now := time.Now()
	for k, f := range s.Files {
		if now.After(f.ExpiresAt) {
			delete(s.Files, k)
		}
	}
	for k, a := range s.Archives {
		if now.After(a.ExpiresAt) {
			delete(s.Archives, k)
		}
	}
}

func cleanDownloadStore() {
	storeStoreMutex.Lock()
	defer storeStoreMutex.Unlock()
	for i, s := range storeStore {
		cleanStore(s)
		if len(s.Files) == 0 && len(s.Archives) == 0 {
			delete(storeStore, i)
		}
	}
}

// GetStore returns the DownloadStore for the given Instance
func GetStore(domain string) DownloadStore {
	storeStoreMutex.Lock()
	defer storeStoreMutex.Unlock()
	if storeStore == nil {
		storeStore = make(map[string]*memStore)
	}
	store, exists := storeStore[domain]
	if !exists {
		store = &memStore{
			Archives: make(map[string]*Archive),
			Files:    make(map[string]*fileRef),
		}
		storeStore[domain] = store
	}
	return store
}

type memStore struct {
	Mutex    sync.Mutex
	Archives map[string]*Archive
	Files    map[string]*fileRef
}

func (s *memStore) makeSecret() string {
	return hex.EncodeToString(crypto.GenerateRandomBytes(16))
}

func (s *memStore) AddFile(f string) (string, error) {
	fref := &fileRef{
		Path:      f,
		ExpiresAt: time.Now().Add(downloadStoreTTL),
	}
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	cleanStore(s)
	key := s.makeSecret()
	s.Files[key] = fref
	return key, nil
}

func (s *memStore) AddArchive(a *Archive) (string, error) {
	a.ExpiresAt = time.Now().Add(downloadStoreTTL)
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	cleanStore(s)
	key := s.makeSecret()
	s.Archives[key] = a
	return key, nil
}

func (s *memStore) GetFile(k string) (string, error) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	f, ok := s.Files[k]
	if !ok {
		return "", nil
	}
	if time.Now().After(f.ExpiresAt) {
		delete(s.Files, k)
		return "", nil
	}
	return f.Path, nil
}

func (s *memStore) GetArchive(k string) (*Archive, error) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	a, ok := s.Archives[k]
	if !ok {
		return nil, nil
	}
	if time.Now().After(a.ExpiresAt) {
		delete(s.Files, k)
		return nil, nil
	}
	return a, nil
}
