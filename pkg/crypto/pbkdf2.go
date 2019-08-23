package crypto

import (
	"crypto/sha256"
	"encoding/base64"

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
	key := pbkdf2.Key(password, salt, iter, hashedPassLen, sha256.New)
	hashed := pbkdf2.Key(key, password, 1, hashedPassLen, sha256.New)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(hashed)))
	base64.StdEncoding.Encode(encoded, hashed)
	return encoded
}
