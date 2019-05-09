package dispers

/*
Describer is a struct used by DAs on different layers to communicate on the shape
of the data they dealt with. 
*/

type Describer struct {
	Dataset          string    `json:"dataset,omitempty"`
  Preprocess       string    `json:"preprocess,omitempty"`
  Standardization  string    `json:"standardization,omitempty"`
  Shape            []int64   `json:"shape,omitempty"`
  Labels           []string  `json:"fakelabels,omitempty"`
}

// NewDescriberInterface returns a DescriberInterface with the specified values.
func NewDescriber(dataset string, preprocess string, standardization string, shape []int64, labels []string) Describer {
	return Describer{
  	Dataset: dataset,
    Preprocess: preprocess,
    Standardization: standardization,
    Shape: shape,
    Labels: labels,
	}
}
