package sharings

import "github.com/cozy/cozy-stack/pkg/oauth"

// Recipient is a struct describing a sharing recipient
type Recipient struct {
	RID      string `json:"_id,omitempty"`
	RRev     string `json:"_rev,omitempty"`
	Mail     string `json:"mail"`
	Url      string `json:"url"`
	ClientID oauth.Client
}

// Recipients is a set of recipient
type Recipients []Recipient
