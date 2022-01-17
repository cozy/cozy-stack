package sharing

// Groups contains the information about a group of contacts that have been
// added as recipient to a sharing.
type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	On   string `json:"on"` // The instance where the group has been added
}
