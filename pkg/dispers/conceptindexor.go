package dispers

import (
  	"github.com/cozy/echo"
    "github.com/cozy/cozy-stack/pkg/prefixer"
    "github.com/cozy/cozy-stack/pkg/couchdb"
)


type conceptDoc struct {
	conceptID     string         `json:"_id,omitempty"`
	conceptRev    string         `json:"_rev,omitempty"`
  concept       string         `json:"concept,omitempty"`
  key           string         `json:"key,omitempty"`
  hash          string         `json:"hash,omitempty"`
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

func hash(concept string) string {
  return concept+"54g5fe45zgr1nefeyf4e5"
}

// Randomly generate a Key
func generateKey() string {
  return "TODOTODO5d4g4r8g4r4gr"
}

func HashMeThat(concept string) echo.Map {

  // TODO : Decrypte concept with private key

  return echo.Map{
    "ok"      : true,
    "concept" : concept, // La valeur pourra être chiffrée
    "hash"    : hash(concept),
    "meta"    : "TODO",
  }
}

func AddConcept(concept string) echo.Map {

  couchdb.EnsureDBExist(prefixer.ConceptIndexorPrefixer, "io.cozy.hashconcept")

  conceptDoc := &conceptDoc{
    conceptID   : "",
    conceptRev  : "",
    concept     : concept,
    key         : generateKey(),
  }

  err := couchdb.CreateDoc(prefixer.ConceptIndexorPrefixer, conceptDoc)

  return echo.Map{
    "ok"      : err==nil,
    "message" : err,
  }
}
