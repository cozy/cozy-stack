package enclave

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/dispers/dispers"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/echo"
)

type dataAggregation struct {
	Input  dispers.DescribeData
	Output dispers.DescribeData
	Data   string
	DocID  string // Doc where will be saved the process
}

// This Doctype will be used to save the aggregation process in memory
// It will be usefull if one API has to work serveral times
type DataAggrDoc struct {
	DataAggrDocID  string          `json:"_id,omitempty"`
	DataAggrDocRev string          `json:"_rev,omitempty"`
	Input          dispers.InputDA `json:"input,omitempty"`
}

func (t *DataAggrDoc) ID() string {
	return t.DataAggrDocID
}

func (t *DataAggrDoc) Rev() string {
	return t.DataAggrDocRev
}

func (t *DataAggrDoc) DocType() string {
	return "io.cozy.aggregation"
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

// NewNewDataAggregation returns a DataAggregation object with the specified values.
func NewDataAggregation(inputDA dispers.InputDA) *dataAggregation {
	// Doc's creation in CouchDB
	couchdb.EnsureDBExist(prefixer.DataAggregatorPrefixer, "io.cozy.aggregation")

	doc := &DataAggrDoc{
		DataAggrDocID:  "",
		DataAggrDocRev: "",
		Input:          inputDA,
	}

	couchdb.CreateDoc(prefixer.DataAggregatorPrefixer, doc)

	// Start an AsyncTask

	return &dataAggregation{
		Input: inputDA.InputType,
		Data:  inputDA.InputData,
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
			"message": err}
	}

	return echo.Map{"outcome": "ok",
		"state":    "Training",
		"metadata": "oh oh oh oh"}

}
