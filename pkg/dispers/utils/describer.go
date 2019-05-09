package utils

type DescriberInterface interface {
  Dataset() string
  Preprocess() string
  Standardization() string
  Shape() []int64
  Labels() []string
}

type describer struct {
	dataset string
  preprocess string
  standardization string
  shape []int64
  labels []string
}

// NewDescriberInterface returns a DescriberInterface with the specified values.
func NewDescriber(dataset string, preprocess string, standardization string, shape []int64, labels []string) DescriberInterface {
	return &describer{
  	dataset: dataset,
    preprocess: preprocess,
    standardization: standardization,
    shape: shape,
    labels: labels,
	}
}

func (d *describer) Dataset() string { return d.dataset }

func (d *describer) Preprocess() string { return d.preprocess }

func (d *describer) Standardization() string { return d.standardization }

func (d *describer) Shape() []int64 { return d.shape }

func (d *describer) Labels() []string { return d.labels }
