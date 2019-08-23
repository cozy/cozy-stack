package crypto

import (
	"crypto/sha256"

	"golang.org/x/crypto/pbkdf2"
)

// DefaultPBKDF2Iterations is the number of iterations used to hash the
// passphrase on the client-side with the PBKDF2 algorithm.
// TODO 100K is recommended, but it is currently only 10K as 100K may be too
// much in Edge. We should test that!
const DefaultPBKDF2Iterations = 10000

// hashedPassLen is the length of the hashed password (in bytes).
const hashedPassLen = 32

// HashPassWithPBKDF2 will hash a password with the PBKDF2 algorithm and same
// parameters as it's done in client side.
func HashPassWithPBKDF2(password, salt []byte, iter int) []byte {
	return pbkdf2.Key(password, salt, iter, hashedPassLen, sha256.New)
}
