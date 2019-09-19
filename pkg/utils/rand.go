package utils

import (
	"io"
	"math/rand"
)

type randGen struct {
	rand.Rand
}

// NewSeededRand returns a random bytes reader initialized with the given seed.
func NewSeededRand(seed int64) io.Reader {
	src := rand.NewSource(seed)
	return &randGen{
		Rand: *rand.New(src),
	}
}

func (r *randGen) Read(p []byte) (n int, err error) {
	for i := 0; i < len(p); i++ {
		p[i] = byte(r.Rand.Intn(255))
	}
	return len(p), nil
}
