package crypto

import (
	"crypto/rand"
	"io"
	"time"
)

// GenerateRandomBytes returns securely generated random bytes. It will return
// an error if the system's secure random number generator fails to function
// correctly, in which case the caller should not continue.
func GenerateRandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic(err)
	}
	return b
}

// Timestamp returns the current timestamp, in seconds.
func Timestamp() int64 {
	return time.Now().UTC().Unix()
}
