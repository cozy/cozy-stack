package dispers

import "errors"

/*
*
Queries' Input & Output
*
*/
type Query struct {
	Query string `json:"query,omitempty"`
	OAuth string `json:"oauth,omitempty"`
}

/*
*
Concept Indexors' Input & Output
*
*/
type OutputCI struct {
	Outcome string `json:"ok,omitempty"`
	Hash    string `json:"hash,omitempty"`
}

/*
*
Target Finders' Input & Output
*
*/
type Operation interface {
	Compute(list map[string][]string) ([]string, error)
}

type Single struct {
	Value string `json:"value,omitempty"`
}

func (s *Single) Compute(list map[string][]string) ([]string, error) {
	val, ok := list[s.Value]
	if !ok {
		return []string{}, errors.New("Unknown concept")
	}
	return val, nil
}

func union(a, b []string) []string {
	m := make(map[string]bool)

	for _, item := range a {
		m[item] = true
	}

	for _, item := range b {
		if _, ok := m[item]; !ok {
			a = append(a, item)
		}
	}
	return a
}

type Union struct {
	ValueA Operation `json:"value_a,omitempty"`
	ValueB Operation `json:"value_b,omitempty"`
}

func (u *Union) Compute(list map[string][]string) ([]string, error) {
	a, err := u.ValueA.Compute(list)
	if err != nil {
		return []string{}, err
	}
	b, err := u.ValueB.Compute(list)
	if err != nil {
		return []string{}, err
	}
	return union(a, b), nil
}

func intersection(a, b []string) (c []string) {
	m := make(map[string]bool)

	for _, item := range a {
		m[item] = true
	}

	for _, item := range b {
		if _, ok := m[item]; ok {
			c = append(c, item)
		}
	}
	return
}

type Intersection struct {
	ValueA Operation `json:"value_a,omitempty"`
	ValueB Operation `json:"value_b,omitempty"`
}

func (i *Intersection) Compute(list map[string][]string) ([]string, error) {
	a, err := i.ValueA.Compute(list)
	if err != nil {
		return []string{}, err
	}
	b, err := i.ValueB.Compute(list)
	if err != nil {
		return []string{}, err
	}
	return intersection(a, b), nil
}

type InputTF struct {
	ListsOfAddresses map[string][]string `json:"concepts,omitempty"`
	TargetProfile    Operation           `json:"operation,omitempty"`
}

type OutputTF struct {
}

/*
*
Targets' Input & Output
*
*/
type InputT struct {
	Localquery       string   `json:"localquery,omitempty"`
	ListsOfAddresses []string `json:"Addresses,omitempty"`
}

type OutputT struct {
	Queries []Query `json:"queries,omitempty"`
}

/*
*
Stacks' Input & Output
*
*/
type OutputStack struct {
}

/*
*
Data Aggregators' Input & Output
*
*/

/*
DescribeData is a struct used by DAs on different layers to communicate on the shape
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
