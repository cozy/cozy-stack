package utils

import (
	"math/rand"
	"time"
)

var urand *rand.Rand

func init() {
	urand = rand.New(rand.NewSource(time.Now().UTC().UnixNano()))
}

// RandomString returns a string of random alpha characters of the specified
// length.
func RandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	lenLetters := len(letters)
	for i := 0; i < n; i++ {
		b[i] = letters[urand.Intn(lenLetters)]
	}
	return string(b)
}
