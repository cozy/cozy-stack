package enclave

import (

  	"github.com/cozy/echo"
  	"github.com/cozy/cozy-stack/pkg/dispers/dispers"
    "github.com/cozy/cozy-stack/pkg/couchdb"
    "github.com/cozy/cozy-stack/pkg/prefixer"
)

type dataAggregation struct {
    Input       dispers.Describer
    Output      dispers.Describer
    Data        string
    DocID       string // Doc where will be saved the process
}

type InputDA struct {
    Input_type dispers.Describer    `json:"type,omitempty"`
    Input_data string             `json:"data,omitempty"`
}

// This Doctype will be used to save the aggregation process in memory
// It will be usefull if one API has to work serveral times
type DataAggrDoc struct {
	dataAggrDocID  string  `json:"_id,omitempty"`
	dataAggrDocRev string  `json:"_rev,omitempty"`
  Input          InputDA `json:"input,omitempty"`
}

func (t *DataAggrDoc) ID() string {
	return t.dataAggrDocID
}

func (t *DataAggrDoc) Rev() string {
	return t.dataAggrDocRev
}

func (t *DataAggrDoc) DocType() string {
	return "io.cozy.aggregation"
}

func (t *DataAggrDoc) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

func (t *DataAggrDoc) SetID(id string) {
	t.dataAggrDocID = id
}

func (t *DataAggrDoc) SetRev(rev string) {
	t.dataAggrDocRev = rev
}

// NewNewDataAggregation returns a DataAggregation object with the specified values.
func NewDataAggregation(inputDA InputDA) *dataAggregation {
  // Doc's creation in CouchDB
  couchdb.EnsureDBExist(prefixer.DataAggregatorPrefixer, "io.cozy.aggregation")

  doc := &DataAggrDoc{
    dataAggrDocID: "",
    dataAggrDocRev: "",
    Input: inputDA,
  }

 couchdb.CreateDoc(prefixer.DataAggregatorPrefixer, doc)

  // Start an AsyncTask

	return &dataAggregation{
    Input: inputDA.Input_type,
    Output: dispers.NewDescriber("", "", "", []int64{0}, []string{""}),
    Data: inputDA.Input_data,
    DocID: doc.ID(),
    }
}


// 4 big methods : see them as a lifecycle of the process
// Load and centralized data in a table
func (da *dataAggregation) LoadDataAggregateData() error { return nil }

// Initialize parameters and apply a ML algorithm on the table
// Async
func (da *dataAggregation) TrainStartingFromScratch() error { return nil }

// Retrieve parameters and apply a ML algorithm on the table
// Async
func (da *dataAggregation) Train(id_train string) error { return nil }

// At any time, get the state of the algorithm
func GetStateOrGetResult(id string) echo.Map {
  couchdb.EnsureDBExist(prefixer.DataAggregatorPrefixer, "io.cozy.aggregation")
  fetched := &DataAggrDoc{}
  err := couchdb.GetDoc(prefixer.DataAggregatorPrefixer, "io.cozy.aggregation", id, fetched)
  if err != nil {
      return echo.Map{"outcome": "error",
                    "message": err }
      }

      return echo.Map{"outcome": "ok",
                    "state" : "Training",
                    "metadata" : "oh oh oh oh"}

}
