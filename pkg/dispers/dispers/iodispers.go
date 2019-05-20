package dispers

type Adresses struct {
	ListOfAdresses []string `json:"adresses,omitempty"`
}

type InputTF struct {
	ListOfConcepts []Adresses `json:"concepts,omitempty"`
}

type InputT struct {
	Localquery      string   `json:"localquery,omitempty"`
	ListsOfAdresses []string `json:"adresses,omitempty"`
}

type Query struct {
	Query string `json:"query,omitempty"`
	OAuth string `json:"oauth,omitempty"`
}

type OutputT struct {
	Queries []Query `json:"queries,omitempty"`
}

/*
Describer is a struct used by DAs on different layers to communicate on the shape
of the data they dealt with.
*/

type DescribeData struct {
	Dataset         string   `json:"dataset,omitempty"`
	Preprocess      string   `json:"preprocess,omitempty"`
	Standardization string   `json:"standardization,omitempty"`
	Shape           []int64  `json:"shape,omitempty"`
	Labels          []string `json:"fakelabels,omitempty"`
}

type InputDA struct {
	InputType DescribeData `json:"type,omitempty"`
	InputData string       `json:"data,omitempty"`
}
