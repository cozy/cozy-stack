package dispers

import "encoding/json"

type Query struct {
	Query string `json:"query,omitempty"`
	OAuth string `json:"oauth,omitempty"`
}

type OutputCI struct {
}

func (o *OutputCI) UnmarshalJSON(data []byte) error {
	var v [2]float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	//o.Price = v[0]
	//o.Volume = v[1]
	return nil
}

type Adresses struct {
	ListOfAdresses []string `json:"adresses,omitempty"`
}

type InputTF struct {
	ListOfConcepts []Adresses `json:"concepts,omitempty"`
}

type OutputTF struct {
}

func (o *OutputTF) UnmarshalJSON(data []byte) error {
	var v [2]float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	//o.Price = v[0]
	//o.Volume = v[1]
	return nil
}

type InputT struct {
	Localquery      string   `json:"localquery,omitempty"`
	ListsOfAdresses []string `json:"adresses,omitempty"`
}

type OutputT struct {
	Queries []Query `json:"queries,omitempty"`
}

func (o *OutputT) UnmarshalJSON(data []byte) error {
	var v [2]float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	//o.Price = v[0]
	//o.Volume = v[1]
	return nil
}

type OutputStack struct {
}

func (o *OutputStack) UnmarshalJSON(data []byte) error {
	var v [2]float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	//o.Price = v[0]
	//o.Volume = v[1]
	return nil
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

type OutputDA struct {
}

func (o *OutputDA) UnmarshalJSON(data []byte) error {
	var v [2]float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	//o.Price = v[0]
	//o.Volume = v[1]
	return nil
}
