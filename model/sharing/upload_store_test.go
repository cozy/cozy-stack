package sharing

import "testing"

func TestUploadStoreIsShared(t *testing.T) {
	if (&memStore{}).IsShared() {
		t.Fatal("memory upload store must not be shared")
	}
	if !(&redisStore{}).IsShared() {
		t.Fatal("redis upload store must be shared")
	}
}
