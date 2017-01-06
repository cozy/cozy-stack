package utils

import (
	"math/rand"
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
	for i := 0; i < n; i++ {
		go func() {
			RandomString(10)
			wg.Done()
		}()
	}
	wg.Wait()
}
