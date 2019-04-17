package dispers.actors

import "github.com/cozy/cozy-stack/pkg/crypto" // to hash and to communicate

type ConceptIndexor struct {
  // Concept
  // Haché d'un concept
}

// New returns a new blank ConceptIndexor.
func New() *ConceptIndexor {
	return &ConceptIndexor{

		},
	}
}

func (c *ConceptIndexor) getHashConcept() string{

}
