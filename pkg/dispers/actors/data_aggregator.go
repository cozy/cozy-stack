package actors

//import "github.com/cozy/cozy-stack/pkg/ml"
//import "github.com/cozy/cozy-stack/pkg/crypto" // to communicate

type DataAggregator struct {
  // New parameters - returned to Stack / MDA
  // Old paremeters - initiated when DA is constructed
  // Data to aggregate
  // Algo ML
}

// New returns a new blank DataAggregator.
func NewDA() *DataAggregator {
	return &DataAggregator{
	}
}

func (da *DataAggregator) aggregate() string{
	return ""
}

func (da *DataAggregator) getParameters() string{
	return ""
}
