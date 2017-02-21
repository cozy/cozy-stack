package sharings

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

// Sharing contains all the information about a sharing
type Sharing struct {
	SID         string `json:"_id,omitempty"`
	SRev        string `json:"_rev,omitempty"`
	Type        string `json:"type,omitempty"`
	Owner       bool   `json:"owner"`
	Desc        string `json:"desc,omitempty"`
	SharingID   string `json:"sharing_id,omitempty"`
	SharingType string `json:"sharing_type"`

	Permissions *permissions.Set    `json:"permissions,omitempty"`
	SRecipients []*SharingRecipient `json:"recipients,omitempty"`
}

// SharingRecipient contains the information about a recipient for a sharing
type SharingRecipient struct {
	Status       string
	AccessToken  string
	RefreshToken string

	RefRecipient jsonapi.ResourceIdentifier `json:"recipient,omitempty"`

	recipient *Recipient
}

// ID returns the sharing qualified identifier
func (s *Sharing) ID() string { return s.SID }

// Rev returns the sharing revision
func (s *Sharing) Rev() string { return s.SRev }

// DocType returns the sharing document type
func (s *Sharing) DocType() string { return consts.Sharings }

// SetID changes the sharing qualified identifier
func (s *Sharing) SetID(id string) { s.SID = id }

// SetRev changes the sharing revision
func (s *Sharing) SetRev(rev string) { s.SRev = rev }

// Links implements jsonapi.Doc
func (s *Sharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharing/" + s.SID}
}

// Recipients returns the sharing recipients
func (s *Sharing) Recipients(db couchdb.Database) ([]*SharingRecipient, error) {
	var sRecipients []*SharingRecipient

	for _, sRec := range s.SRecipients {
		recipient, err := GetRecipient(db, sRec.RefRecipient.ID)
		if err != nil {
			return nil, err
		}
		sRec.recipient = recipient
		sRecipients = append(sRecipients, sRec)
	}

	s.SRecipients = sRecipients
	return sRecipients, nil
}

// Relationships is part of the jsonapi.Object interface
// It is used to generate the recipients relationships
func (s *Sharing) Relationships() jsonapi.RelationshipMap {
	l := len(s.SRecipients)
	i := 0

	data := make([]jsonapi.ResourceIdentifier, l)
	for _, rec := range s.SRecipients {
		r := rec.recipient
		data[i] = jsonapi.ResourceIdentifier{ID: r.ID(), Type: r.DocType()}
		i++
	}
	contents := jsonapi.Relationship{Data: data}
	return jsonapi.RelationshipMap{"recipients": contents}
}

// Included is part of the jsonapi.Object interface
func (s *Sharing) Included() []jsonapi.Object {
	var included []jsonapi.Object
	for _, rec := range s.SRecipients {
		r := rec.recipient
		included = append(included, r)
	}
	return included
}

// GetRecipient returns the Recipient stored in database from a given ID
func GetRecipient(db couchdb.Database, recID string) (*Recipient, error) {
	doc := &Recipient{}
	err := couchdb.GetDoc(db, consts.Recipients, recID, doc)
	if couchdb.IsNotFoundError(err) {
		err = ErrRecipientDoesNotExist
	}
	return doc, err
}

// CheckSharingCreation initializes and check some sharing fields at creation
func CheckSharingCreation(db couchdb.Database, sharing *Sharing) error {

	sharingType := sharing.SharingType
	if sharingType != consts.OneShotSharing &&
		sharingType != consts.MasterSlaveSharing &&
		sharingType != consts.MasterMasterSharing {
		return ErrBadSharingType
	}

	sRecipients, err := sharing.Recipients(db)
	if err != nil {
		return err
	}
	for _, sRec := range sRecipients {
		sRec.Status = consts.PendingSharingStatus
	}

	sharing.Owner = true
	sharing.SharingID = utils.RandomString(32)

	return nil
}

// Create inserts a Sharing document in database
func Create(db couchdb.Database, doc *Sharing) error {
	err := couchdb.CreateDoc(db, doc)
	if err != nil {
		return err
	}
	return nil
}
