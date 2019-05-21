package enclave

import (
	"errors"
	"strings"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

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

func addSalt(concept string) error {

	ConceptDoc := &ConceptDoc{
		ConceptID:  "",
		ConceptRev: "",
		Concept:    concept,
		Salt:       string(crypto.GenerateRandomBytes(5)),
	}

	return couchdb.CreateDoc(prefixer.ConceptIndexorPrefixer, ConceptDoc)
}

func getSalt(concept string) (string, error) {

	salt := ""
	err := couchdb.DefineIndex(prefixer.ConceptIndexorPrefixer, mango.IndexOnFields("io.cozy.hashconcept", "concept-index", []string{"concept"}))
	if err != nil {
		return salt, err
	}

	var out []ConceptDoc
	req := &couchdb.FindRequest{Selector: mango.Equal("concept", concept)}
	err = couchdb.FindDocs(prefixer.ConceptIndexorPrefixer, "io.cozy.hashconcept", req, out)
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
func hash(str string) (string, error) {

	scrypt, err := crypto.GenerateFromPassphrase([]byte(str))
	if err != nil {
		return "", err
	}

	return string(scrypt), err
}

func isConceptExisting(concept string) (bool, error) {

	var out []ConceptDoc
	req := &couchdb.FindRequest{Selector: mango.Equal("concept", concept)}
	err := couchdb.FindDocs(prefixer.ConceptIndexorPrefixer, "io.cozy.hashconcept", req, out)
	if err != nil {
		return false, err
	}

	if len(out) > 0 {
		return true, nil
	}

	return false, nil
}

/*
DeleteConcept is used to delete a salt in ConceptIndexor Database. It has to be
used to update a salt.
*/
func DeleteConcept(encryptedConcept string) error {

	// TODO: Decrypte concept with private key
	concept := encryptedConcept

	// TODO: Delete document in database
	var out []ConceptDoc
	req := &couchdb.FindRequest{Selector: mango.Equal("concept", concept)}
	err := couchdb.FindDocs(prefixer.ConceptIndexorPrefixer, "io.cozy.hashconcept", req, out)
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

	return err
}

/*
HashMeThat is used to get a concept's salt. If the salt is absent from CI database
the function creates the salt and notify the user that the salt has just been created
*/
func HashMeThat(encryptedConcept string) (string, error) {
	couchdb.EnsureDBExist(prefixer.ConceptIndexorPrefixer, "io.cozy.hashconcept")

	// TODO: Decrypte concept with private key
	concept := encryptedConcept

	isExisting, err := isConceptExisting(concept)
	if err != nil {
		return "", err
	}

	if isExisting {
		// Write in Metadata that concept has been retrieved
	} else {
		err = addSalt(concept)
		if err != nil {
			return "", err
		}
		// Write in Metadata that concept has been created
	}

	// Get salt with hash(concept)
	hashedConcept, err := hash(concept)
	if err != nil {
		return "", err
	}
	salt, err := getSalt(hashedConcept)
	if err != nil {
		return "", err
	}

	// Merge concept and salt, then hash
	justHashed, err := hash(strings.Join([]string{concept, salt}, ""))

	return justHashed, err
}
