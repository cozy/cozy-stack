package utils

// This script is defining metadata's interface. It will be use to define metadata for each api/treatment
type Metadata interface {
    Name()        string
    Date()        string
    Outcome()     bool
    Description() string
}

type metadata struct {
    date        string   `json:"date,omitempty"`
    description string   `json:"description,omitempty"`
    name        string   `json:"name,omitempty"`
    outcome     bool     `json:"outcome,omitempty"`
}

func NewMetadata(name string, description string, date string, outcome bool) Metadata {
	return &metadata{
		date: date,
    description: description,
    outcome: outcome,
    name: name,
	}
}

func (m *metadata) Date() string { return m.date }

func (m *metadata) Description() string { return m.description }

func (m *metadata) Outcome() bool { return m.outcome }

func (m *metadata) Name() string { return m.name }
