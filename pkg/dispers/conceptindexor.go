package enclave

import (
	"errors"

	"golang.org/x/crypto/scrypt"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

const defaultN = 32768
const defaultR = 8
const defaultP = 1

// hash length
const defaultDkLen = 32

// salt length
var defaultSalt = "CozyCloud"

/*
ConceptDoc is used to save a concept's salt into Concept Indexor's database
*/
type ConceptDoc struct {
	ConceptID  string `json:"_id,omitempty"`
	ConceptRev string `json:"_rev,omitempty"`
	Concept    string `json:"concept,omitempty"`
	Salt       string `json:"salt,omitempty"`
}

// ID is used to get ID
func (t *ConceptDoc) ID() string {
	return t.ConceptID
}

// Rev is used to get Rev
func (t *ConceptDoc) Rev() string {
	return t.ConceptRev
}

// DocType is used to get DocType
func (t *ConceptDoc) DocType() string {
	return "io.cozy.hashconcept"
}

// Clone is used to create another ConceptDoc from this ConceptDoc
func (t *ConceptDoc) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

// SetID is used to set the Doc ID
func (t *ConceptDoc) SetID(id string) {
	t.ConceptID = id
}

// SetRev is used to set Rev
func (t *ConceptDoc) SetRev(rev string) {
	t.ConceptRev = rev
}

// AddSalt does not check is one salt is already existing
func AddSalt(hashedConcept string) error {

	ConceptDoc := &ConceptDoc{
		ConceptID:  "",
		ConceptRev: "",
		Concept:    hashedConcept,
		Salt:       string(crypto.GenerateRandomBytes(5)),
	}

	return couchdb.CreateDoc(prefixer.ConceptIndexorPrefixer, ConceptDoc)
}

func GetSalt(hashedConcept string) (string, error) {

	salt := ""
	err := couchdb.DefineIndex(prefixer.ConceptIndexorPrefixer, mango.IndexOnFields("io.cozy.Hashconcept", "concept-index", []string{"concept"}))
	if err != nil {
		return salt, err
	}

	var out []ConceptDoc
	req := &couchdb.FindRequest{Selector: mango.Equal("concept", hashedConcept)}
	err = couchdb.FindDocs(prefixer.ConceptIndexorPrefixer, "io.cozy.Hashconcept", req, &out)
	if err != nil {
		return salt, err
	}

	if len(out) == 1 {
		salt = out[0].Salt
	} else if len(out) == 0 {
		// TODO: Creation an error
		return "", errors.New("Concept Indexor : no existing salt for this concept")
	} else {
		return "", errors.New("Concept Indexor : several salts for this concept")
	}

	return salt, err
}

// TODO: Implements bcrypt or argon instead of scrypt
func Hash(str string, saltIn string) (string, error) {

	salt := saltIn
	if saltIn == "" {
		salt = defaultSalt
	}

	// scrypt the cleartext passphrase with the same parameters
	other, err := scrypt.Key([]byte(str), []byte(salt), defaultN, defaultR, defaultP, defaultDkLen)
	if err != nil {
		return "", err
	}

	return string(other), err
}

func IsConceptExisting(hashedConcept string) (bool, error) {

	err := couchdb.DefineIndex(prefixer.ConceptIndexorPrefixer, mango.IndexOnFields("io.cozy.Hashconcept", "concept-index", []string{"concept"}))
	if err != nil {
		return false, err
	}

	var out []ConceptDoc
	req := &couchdb.FindRequest{Selector: mango.Equal("concept", hashedConcept)}
	err = couchdb.FindDocs(prefixer.ConceptIndexorPrefixer, "io.cozy.Hashconcept", req, &out)
	if err != nil {
		return false, err
	}

	if len(out) > 0 {
		return true, nil
	}

	return false, nil
}

func DeleteSalt(hashedConcept string) error {
	// Delete document in database
	var out []ConceptDoc
	req := &couchdb.FindRequest{Selector: mango.Equal("concept", hashedConcept)}
	err := couchdb.FindDocs(prefixer.ConceptIndexorPrefixer, "io.cozy.Hashconcept", req, &out)
	if err != nil {
		return err
	}

	if len(out) == 0 {
		return errors.New("No concept to delete")
	}

	// Delete every doc that match concept
	for _, element := range out {
		err = couchdb.DeleteDoc(prefixer.ConceptIndexorPrefixer, &element)
		if err != nil {
			return err
		}
	}

	return nil
}

/*
DeleteConcept is used to delete a salt in ConceptIndexor Database. It has to be
used to update a salt.
*/
func DeleteConcept(encryptedConcept string) error {

	// TODO: Decrypte concept with private key
	concept := encryptedConcept

	hashedConcept, err := Hash(concept, "")
	if err != nil {
		return err
	}

	DeleteSalt(hashedConcept)

	return err
}

/*
HashMeThat is used to get a concept's salt. If the salt is absent from CI database
the function creates the salt and notify the user that the salt has just been created
*/
func HashMeThat(encryptedConcept string) (string, error) {
	couchdb.EnsureDBExist(prefixer.ConceptIndexorPrefixer, "io.cozy.Hashconcept")

	// TODO: Decrypte concept with private key
	concept := encryptedConcept

	// Get salt with Hash(concept)
	hashedConcept, err := Hash(concept, "")
	if err != nil {
		return "", err
	}

	isExisting, err := IsConceptExisting(hashedConcept)
	if err != nil {
		return "", err
	}

	if isExisting {
		// Write in Metadata that concept has been retrieved
	} else {
		err = AddSalt(hashedConcept)
		if err != nil {
			return "", err
		}
		// Write in Metadata that concept has been created
	}

	salt, err := GetSalt(hashedConcept)
	if err != nil {
		return "", err
	}

	// Merge concept and salt, then Hash
	justHashed, err := Hash(concept, salt)

	return justHashed, err
}
