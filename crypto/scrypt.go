package crypto

// Params describes the input parameters to the scrypt
import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/crypto/scrypt"
)

// The code below is heavily inspired by https://github.com/elithrar/simple-scrypt

// Scrypt params
// TODO audit this parameters taken from simple-scrypt DefaultParams
const n = 16384
const r = 8
const p = 1

// hash length
const dkLen = 32

// salt length
const saltLen = 16

// Errors
var (
	ErrInvalidHash                 = errors.New("Invalid hash format")
	ErrMismatchedHashAndPassphrase = errors.New("hash and password are different")
	ErrNoUpdateNeeded              = errors.New("hash already has correct parameters")
)

var sep = []byte("$")

type scryptHash struct {
	n    int
	r    int
	p    int
	salt []byte
	dk   []byte
}

func (h *scryptHash) UnmarshalText(hashbytes []byte) error {
	// Decode existing hash, retrieve params and salt.
	vals := bytes.Split(hashbytes, sep)
	// "scrypt", P, N, R, salt, scrypt derived key
	if len(vals) != 6 {
		return ErrInvalidHash
	}
	if string(vals[0]) != "scrypt" {
		return ErrInvalidHash
	}

	var err error

	h.n, err = strconv.Atoi(string(vals[1]))
	if err != nil {
		return ErrInvalidHash
	}

	h.r, err = strconv.Atoi(string(vals[2]))
	if err != nil {
		return ErrInvalidHash
	}

	h.p, err = strconv.Atoi(string(vals[3]))
	if err != nil {
		return ErrInvalidHash
	}

	h.salt = make([]byte, hex.DecodedLen(len(vals[4])))
	hex.Decode(h.salt, vals[4])
	if err != nil {
		return ErrInvalidHash
	}

	h.dk = make([]byte, hex.DecodedLen(len(vals[5])))
	hex.Decode(h.dk, vals[5])
	if err != nil {
		return ErrInvalidHash
	}

	return nil
}

func (h *scryptHash) MarshalText() ([]byte, error) {
	s := fmt.Sprintf("scrypt$%d$%d$%d$%x$%x", h.n, h.r, h.p, h.salt, h.dk)
	return []byte(s), nil
}

func (h *scryptHash) Compare(passphrase []byte) error {
	// scrypt the cleartext passphrase with the same parameters and salt
	other, err := scrypt.Key(passphrase, h.salt, h.n, h.r, h.p, len(h.dk))
	if err != nil {
		return err
	}

	// Constant time comparison
	if subtle.ConstantTimeCompare(h.dk, other) == 1 {
		return nil
	}

	return ErrMismatchedHashAndPassphrase
}

func (h *scryptHash) NeedUpdate() bool {
	return h.n != n || h.p != p || h.r != r || len(h.salt) != saltLen || len(h.dk) != dkLen
}

// GenerateFromPassphrase returns the derived key of the passphrase using the
// parameters provided. The parameters are prepended to the derived key and
// separated by the "$" character (0x24).
// If the parameters provided are less than the minimum acceptable values,
// an error will be returned.
func GenerateFromPassphrase(passphrase []byte) ([]byte, error) {
	var h = &scryptHash{n: n, r: r, p: p}
	var err error

	h.salt, err = GenerateRandomBytes(saltLen)
	if err != nil {
		// Random generator is broken
		return nil, err
	}

	// scrypt.Key returns the raw scrypt derived key.
	h.dk, err = scrypt.Key(passphrase, h.salt, h.n, h.r, h.p, dkLen)
	if err != nil {
		return nil, err
	}

	return h.MarshalText()
}

// CompareHashAndPassphrase compares a derived key with the possible cleartext
// equivalent. The parameters used in the provided derived key are used.
// The comparison performed by this function is constant-time. It returns nil
// on success, and an error if the derived keys do not match.
func CompareHashAndPassphrase(hash []byte, passphrase []byte) error {
	var h = &scryptHash{}
	var err error

	err = h.UnmarshalText(hash)
	if err != nil {
		return err
	}

	return h.Compare(passphrase)
}

// UpdateHash returns a new hash for this passphrase to be stored in db only
// if the parameters have changed.
func UpdateHash(hash []byte, passphrase []byte) ([]byte, error) {
	var h = &scryptHash{}
	var err error

	err = h.UnmarshalText(hash)
	if err != nil {
		return nil, err
	}

	err = h.Compare(passphrase)
	if err != nil {
		return nil, err
	}

	if !h.NeedUpdate() {
		return nil, ErrNoUpdateNeeded
	}

	return GenerateFromPassphrase(passphrase)
}
