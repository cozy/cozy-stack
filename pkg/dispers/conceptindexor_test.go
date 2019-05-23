package enclave

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/stretchr/testify/assert"
)

func TestConceptIndexor(t *testing.T) {

	// Generate fake concept
	conceptTest := "FrançoisEtPaul"
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))

	// Test if concept exist (conceptTestRandom is supposed to be new)
	b, err := isConceptExisting(conceptTestRandom)
	assert.NoError(t, err)
	assert.Equal(t, b, false)

	// Create new conceptDoc with HashMeThat
	hashed, err := HashMeThat(conceptTest)
	_, errb := HashMeThat(conceptTestRandom)
	assert.NoError(t, err)
	assert.NoError(t, errb)

	// Test if concept exist using hash and isConceptExisting
	hashedConceptTest, err := hash(conceptTest, "")
	hashedConceptRandom, err2 := hash(conceptTestRandom, "")
	assert.NoError(t, err)
	assert.NoError(t, err2)
	a, err2 := isConceptExisting(hashedConceptTest)
	b2, errb := isConceptExisting(hashedConceptRandom)
	assert.NoError(t, err2)
	assert.NoError(t, errb)
	assert.Equal(t, a, true)
	assert.Equal(t, b2, true)

	// Check a second HashMeThat
	hashed2, err := HashMeThat(conceptTest)
	assert.NoError(t, err)
	assert.Equal(t, hashed, hashed2)

	// Create twice the same salt and check if there is error when getting salt
	errAdd := addSalt(hashedConceptTest)
	assert.NoError(t, errAdd)
	_, errGet := getSalt(hashedConceptTest)
	assert.Error(t, errGet)
	_, errGetb := getSalt(hashedConceptRandom)
	assert.NoError(t, errGetb)

	// deleteConcept
	errD := DeleteConcept(conceptTest)
	errDb := DeleteConcept(conceptTestRandom)
	assert.NoError(t, errD)
	assert.NoError(t, errDb)

	// Test if concept exist
	ad, err := isConceptExisting(hashedConceptTest)
	bd, errb := isConceptExisting(hashedConceptRandom)
	assert.NoError(t, err)
	assert.NoError(t, errb)
	assert.Equal(t, ad, false)
	assert.Equal(t, bd, false)

}

func TestAddSalt(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))
	errAdd := addSalt(conceptTestRandom)
	assert.NoError(t, errAdd)
}

func TestDeleteSalt(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))
	addSalt(conceptTestRandom)
	err := deleteSalt(conceptTestRandom)
	assert.NoError(t, err)
}

func TestGetSalt(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))
	_ = addSalt(conceptTestRandom)
	_, err := getSalt(conceptTestRandom)
	assert.NoError(t, err)
}

func TestHash(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(10))
	res, err := hash(conceptTestRandom, "")
	assert.NoError(t, err)
	res2, _ := hash(conceptTestRandom, "")
	assert.Equal(t, res, res2)
}

func TestHashMeThat(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(10))
	res, err := HashMeThat(conceptTestRandom)
	assert.NoError(t, err)
	res2, _ := HashMeThat(conceptTestRandom)
	assert.Equal(t, res, res2)
}

func TestHashIsNotDeterministic(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(10))
	hash1, _ := HashMeThat(conceptTestRandom)
	DeleteConcept(conceptTestRandom)
	hash2, _ := HashMeThat(conceptTestRandom)
	DeleteConcept(conceptTestRandom)
	assert.NotEqual(t, hash1, hash2)
}

func TestIsExistantSaltExisting(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))
	addSalt(conceptTestRandom)
	_, err := isConceptExisting(conceptTestRandom)
	assert.NoError(t, err)
}

func TestIsUnexistantSaltExisting(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))
	_, err := isConceptExisting(conceptTestRandom)
	assert.NoError(t, err)
}
