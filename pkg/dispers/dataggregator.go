package dispers

import (
  	"github.com/cozy/cozy-stack/pkg/dispers/utils"
    "github.com/cozy/cozy-stack/pkg/couchdb"
)

// This Doctype will be used to save the aggregation process in memory
// It will be usefull if one API has to work serveral times
type DataAggrDoc struct {
	DataAggrDocID  string `json:"_id,omitempty"`
	DataAggrDocRev string `json:"_rev,omitempty"`
}

func (t *DataAggrDoc) ID() string {
	return t.DataAggrDocID
}

func (t *DataAggrDoc) Rev() string {
	return t.DataAggrDocRev
}

func (t *DataAggrDoc) DocType() string {
	return "io.cozy.dataaggregation"
}

func (t *DataAggrDoc) Clone() couchdb.Doc {
	cloned := *t
	return &cloned
}

func (t *DataAggrDoc) SetID(id string) {
	t.DataAggrDocID = id
}

func (t *DataAggrDoc) SetRev(rev string) {
	t.DataAggrDocRev = rev
}

type dataAggregation struct {
    input utils.DescriberInterface
    output utils.DescriberInterface
    output_json string // Has to be updated by every methods
    data string
    doc DataAggrDoc // Doc where will be saved the process
}

// NewNewDataAggregation returns a DataAggregation object with the specified values.
func NewDataAggregation(input utils.DescriberInterface, data string) *dataAggregation {
  // Créer le doc
  const doc_id = "012"
  const doc_rev = "2-11515"
	return &dataAggregation{
    input: input,
    output: input,
    output_json: "",
    data: data,
    doc: DataAggrDoc{
      DataAggrDocID: doc_id,
      DataAggrDocRev: doc_rev,
    },
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
func (da *dataAggregation) GetStateOrGetResult() string { return da.output_json }
