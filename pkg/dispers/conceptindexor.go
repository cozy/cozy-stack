package enclave

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/echo"
	//"github.com/cozy/cozy-stack/pkg/crypto" utils -> fct to generate random bytes
)

type conceptDoc struct {
	conceptID  string `json:"_id,omitempty"` // ID = hash(concept)
	conceptRev string `json:"_rev,omitempty"`
	salt       string `json:"salt,omitempty"`
}

func (t *conceptDoc) ID() string {
	return t.conceptID
}

func (t *conceptDoc) Rev() string {
	return t.conceptRev
}

func (t *conceptDoc) DocType() string {
	return "io.cozy.hashconcept"
}

func (t *conceptDoc) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

func (t *conceptDoc) SetID(id string) {
	t.conceptID = id
}

func (t *conceptDoc) SetRev(rev string) {
	t.conceptRev = rev
}

func hash(concept string, salt string) string {
	return concept + "54g5fe45zgr1nefeyf4e5"
}

// Randomly generate a salt
func generatesalt() string {
	return "TODOTODO5d4g4r8g4r4gr"
}

func HashMeThat(concept string) echo.Map {

	// TODO : Decrypte concept with private key
	// TODO : get salt with hash(concept)
	salt := "hey"

	return echo.Map{
		"ok":      true,
		"concept": concept, // La valeur pourra être chiffrée
		"hash":    hash(concept, salt),
		"meta":    "TODO",
	}
}

func AddConcept(concept string) echo.Map {

	couchdb.EnsureDBExist(prefixer.ConceptIndexorPrefixer, "io.cozy.hashconcept")
	conceptDoc := &conceptDoc{
		conceptID:  "",
		conceptRev: "",
		salt:       generatesalt(),
	}

	// TODO : Change to create doc with hash(concept) as doc ID
	err := couchdb.CreateDoc(prefixer.ConceptIndexorPrefixer, conceptDoc)

	return echo.Map{
		"ok":      err == nil,
		"message": err,
	}
}
