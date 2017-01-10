package utils

import (
	"math/rand"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomString(t *testing.T) {
	rand.Seed(42)
	s1 := RandomString(10)
	s2 := RandomString(20)

	rand.Seed(42)
	s3 := RandomString(10)
	s4 := RandomString(20)

	assert.Len(t, s1, 10)
	assert.Len(t, s2, 20)
	assert.Len(t, s3, 10)
	assert.Len(t, s4, 20)

	assert.NotEqual(t, s1, s2)
	assert.Equal(t, s1, s3)
	assert.Equal(t, s2, s4)
}

func TestRandomStringConcurrentAccess(t *testing.T) {
	n := 10000
	var wg sync.WaitGroup
	wg.Add(n)

	ms := make(map[string]struct{})
	var mu sync.Mutex

	for i := 0; i < n; i++ {
		go func() {
			s := RandomString(10)
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()
			if _, ok := ms[s]; ok {
				t.Fatal("should be unique strings")
			}
			var q struct{}
			ms[s] = q
		}()
	}
	wg.Wait()
}

func TestFileExists(t *testing.T) {
	exists, err := FileExists("/no/such/file")
	assert.NoError(t, err)
	assert.False(t, exists)
	exists, err = FileExists("/etc/hosts")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestAbsPath(t *testing.T) {
	home := UserHomeDir()
	assert.NotEmpty(t, home)
	assert.Equal(t, home, AbsPath("~"))
	foo := AbsPath("foo")
	wd, _ := os.Getwd()
	assert.Equal(t, wd+"/foo", foo)
	bar := AbsPath("~/bar")
	assert.Equal(t, home+"/bar", bar)
	baz := AbsPath("$HOME/baz")
	assert.Equal(t, home+"/baz", baz)
	qux := AbsPath("/qux")
	assert.Equal(t, "/qux", qux)
	quux := AbsPath("////qux//quux/../quux")
	assert.Equal(t, "/qux/quux", quux)
}
