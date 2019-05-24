package dispers

import (
	"time"
)

// Metadata are written on the confuctor's database. The querier can read those Metadata to know his training's state
type Metadata struct {
	Start       time.Time
	Time        string   `json:"date,omitempty"`
	Host        string   `json:"host,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Outcome     bool     `json:"outcome,omitempty"`
	Error       string   `json:"error,omitempty"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Output      string   `json:"output,omitempty"`
	Duration    string   `json:"duration,omitempty"`
}

// NewMetadata returns a new Metadata object
func NewMetadata(host string, name string, description string, tags []string) Metadata {
	now := time.Now()
	return Metadata{
		Time:        now.String(),
		Start:       now,
		Description: description,
		Name:        name,
		Tags:        tags,
	}
}

// Close finish writting Metadata
func (m *Metadata) Close(msg string, err error) error {
	now := time.Now()
	m.Duration = (now.Sub(m.Start)).String()
	m.Outcome = (err == nil)
	m.Output = msg
	if err != nil {
		m.Error = err.Error()
	}
	return nil
}
