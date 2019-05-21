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
	b, err := IsConceptExisting(conceptTestRandom)
	assert.NoError(t, err)
	assert.Equal(t, b, false)

	// Create new conceptDoc with HashMeThat
	hashed, err := HashMeThat(conceptTest)
	_, errb := HashMeThat(conceptTestRandom)
	assert.NoError(t, err)
	assert.NoError(t, errb)

	// Test if concept exist using Hash and IsConceptExisting
	hashedConceptTest, err := Hash(conceptTest, "")
	hashedConceptRandom, err2 := Hash(conceptTestRandom, "")
	assert.NoError(t, err)
	assert.NoError(t, err2)
	a, err2 := IsConceptExisting(hashedConceptTest)
	b2, errb := IsConceptExisting(hashedConceptRandom)
	assert.NoError(t, err2)
	assert.NoError(t, errb)
	assert.Equal(t, a, true)
	assert.Equal(t, b2, true)

	// Check a second HashMeThat
	hashed2, err := HashMeThat(conceptTest)
	assert.NoError(t, err)
	assert.Equal(t, hashed, hashed2)

	// Create twice the same salt and check if there is error when getting salt
	errAdd := AddSalt(hashedConceptTest)
	assert.NoError(t, errAdd)
	_, errGet := GetSalt(hashedConceptTest)
	assert.Error(t, errGet)
	_, errGetb := GetSalt(hashedConceptRandom)
	assert.NoError(t, errGetb)

	// deleteConcept
	errD := DeleteConcept(conceptTest)
	errDb := DeleteConcept(conceptTestRandom)
	assert.NoError(t, errD)
	assert.NoError(t, errDb)

	// Test if concept exist
	ad, err := IsConceptExisting(hashedConceptTest)
	bd, errb := IsConceptExisting(hashedConceptRandom)
	assert.NoError(t, err)
	assert.NoError(t, errb)
	assert.Equal(t, ad, false)
	assert.Equal(t, bd, false)

}

func TestAddSalt(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))
	errAdd := AddSalt(conceptTestRandom)
	assert.NoError(t, errAdd)
}

func TestDeleteSalt(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))
	AddSalt(conceptTestRandom)
	err := DeleteSalt(conceptTestRandom)
	assert.NoError(t, err)
}

func TestGetSalt(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))
	_ = AddSalt(conceptTestRandom)
	_, err := GetSalt(conceptTestRandom)
	assert.NoError(t, err)
}

func TestHash(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(10))
  res, err := Hash(conceptTestRandom, "")
  assert.NoError(t, err)
  res2, _ := Hash(conceptTestRandom, "")
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
	AddSalt(conceptTestRandom)
	_, err := IsConceptExisting(conceptTestRandom)
	assert.NoError(t, err)
}

func TestIsUnexistantSaltExisting(t *testing.T) {
	conceptTestRandom := string(crypto.GenerateRandomBytes(50))
	_, err := IsConceptExisting(conceptTestRandom)
	assert.NoError(t, err)
}
