package sharings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/oauth"
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

	Permissions      permissions.Set    `json:"permissions,omitempty"`
	RecipientsStatus []*RecipientStatus `json:"recipients,omitempty"`
}

// RecipientStatus contains the information about a recipient for a sharing
type RecipientStatus struct {
	Status       string `json:"status,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`

	RefRecipient jsonapi.ResourceIdentifier `json:"recipient,omitempty"`

	recipient *Recipient
}

// SharingAnswer contains the necessary information to answer a sharing
// request, be it accepted or refused.
// A refusal only contains the mandatory fields: SharingID and ClientID.
// An acceptance contains **everything**.
type SharingAnswer struct {
	SharingID    string `json:"state"`
	ClientID     string `json:"client_id"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
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
	return &jsonapi.LinksList{Self: "/sharings/" + s.SID}
}

// RecStatus returns the sharing recipients status
func (s *Sharing) RecStatus(db couchdb.Database) ([]*RecipientStatus, error) {
	var rStatus []*RecipientStatus

	for _, rec := range s.RecipientsStatus {
		recipient, err := GetRecipient(db, rec.RefRecipient.ID)
		if err != nil {
			return nil, err
		}
		rec.recipient = recipient
		rStatus = append(rStatus, rec)
	}

	s.RecipientsStatus = rStatus
	return rStatus, nil
}

// Recipients returns the sharing recipients
func (s *Sharing) Recipients(db couchdb.Database) ([]*Recipient, error) {
	var recipients []*Recipient

	for _, rec := range s.RecipientsStatus {
		recipient, err := GetRecipient(db, rec.RefRecipient.ID)
		if err != nil {
			return nil, err
		}
		rec.recipient = recipient
		recipients = append(recipients, recipient)
	}

	return recipients, nil
}

// Relationships is part of the jsonapi.Object interface
// It is used to generate the recipients relationships
func (s *Sharing) Relationships() jsonapi.RelationshipMap {
	l := len(s.RecipientsStatus)
	i := 0

	data := make([]jsonapi.ResourceIdentifier, l)
	for _, rec := range s.RecipientsStatus {
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
	for _, rec := range s.RecipientsStatus {
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

// GetSharingRecipientFromClientID returns the Recipient associated with the given clientID
func (s *Sharing) GetSharingRecipientFromClientID(db couchdb.Database, clientID string) (*RecipientStatus, error) {
	for _, recStatus := range s.RecipientsStatus {
		recipient, err := GetRecipient(db, recStatus.RefRecipient.ID)
		if err != nil {
			return nil, err
		}
		recStatus.recipient = recipient

		if recipient.Client.ClientID == clientID {
			return recStatus, nil
		}
	}
	return nil, nil
}

//CheckSharingType returns an error if the sharing type is incorrect
func CheckSharingType(sharingType string) error {
	switch sharingType {
	case consts.OneShotSharing, consts.MasterSlaveSharing, consts.MasterMasterSharing:
		return nil
	}
	return ErrBadSharingType
}

// findSharingRecipient retrieve a sharing recipient from its clientID and sharingID
func findSharingRecipient(db couchdb.Database, sharingID, clientID string) (*Sharing, *RecipientStatus, error) {
	var res []Sharing

	err := couchdb.FindDocs(db, consts.Sharings, &couchdb.FindRequest{
		Selector: mango.Equal("sharing_id", sharingID),
	}, &res)
	if err != nil {
		return nil, nil, err
	}
	if len(res) < 1 {
		return nil, nil, ErrSharingDoesNotExist
	} else if len(res) > 2 {
		return nil, nil, ErrSharingIDNotUnique
	}

	sharing := &res[0]

	sRec, err := sharing.GetSharingRecipientFromClientID(db, clientID)
	if err != nil {
		return nil, nil, err
	}
	if sRec == nil {
		return nil, nil, ErrRecipientDoesNotExist
	}

	return sharing, sRec, nil
}

// SharingRefused handles a rejectedsharing on the sharer side
func SharingRefused(db couchdb.Database, state, clientID string) error {
	sharing, recStatus, err := findSharingRecipient(db, state, clientID)
	if err != nil {
		return err
	}
	recStatus.Status = consts.RefusedSharingStatus
	err = couchdb.UpdateDoc(db, sharing)
	return err
}

// RecipientRefusedSharing executes all the actions induced by a refusal from
// the recipient: the sharing document is deleted and the sharer is informed.
func RecipientRefusedSharing(db couchdb.Database, sharingID, clientID string) error {
	// We get the sharing document through its sharing id…
	var res []Sharing
	err := couchdb.FindDocs(db, consts.Sharings, &couchdb.FindRequest{
		Selector: mango.Equal("sharing_id", sharingID),
	}, &res)
	if err != nil {
		return err
	} else if len(res) < 1 {
		return ErrSharingDoesNotExist
	} else if len(res) > 1 {
		return ErrSharingIDNotUnique
	}
	sharing := &res[0]

	// … and we delete it because it is no longer needed.
	err = couchdb.DeleteDoc(db, sharing)
	if err != nil {
		return err
	}

	// We get the sharer's oauth client so that we can get her Cozy's url
	// through the `ClientURI`.
	sharer := &oauth.Client{}
	err = couchdb.GetDoc(db, consts.OAuthClients, clientID, sharer)
	if err != nil {
		return ErrNoOAuthClient
	}

	// We send the refusal.
	bodyRaw := &SharingAnswer{
		ClientID:  clientID,
		SharingID: sharingID,
	}
	body, _ := json.Marshal(bodyRaw)

	url := fmt.Sprintf("%s/sharings/answer", sharer.ClientURI)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Errorf("[Sharing] The sharer might not have received the answer, she replied with: %s", resp.Status)
		return ErrSharerDidNotReceiveAnswer
	}

	return nil
}

// CreateSharingRequest checks fields integrity and creates a sharing document
// for an incoming sharing request
func CreateSharingRequest(db couchdb.Database, desc, state, sharingType, scope string) (*Sharing, error) {
	if state == "" {
		return nil, ErrMissingState
	}
	if err := CheckSharingType(sharingType); err != nil {
		return nil, err
	}
	if scope == "" {
		return nil, ErrMissingScope
	}
	permissions, err := permissions.UnmarshalScopeString(scope)
	if err != nil {
		return nil, err
	}

	sharing := &Sharing{
		SharingType: sharingType,
		SharingID:   state,
		Permissions: permissions,
		Owner:       false,
		Desc:        desc,
	}

	err = Create(db, sharing)

	return sharing, err
}

// CheckSharingCreation initializes and check some sharing fields at creation
func CheckSharingCreation(db couchdb.Database, sharing *Sharing) error {

	sharingType := sharing.SharingType
	if err := CheckSharingType(sharingType); err != nil {
		return err
	}

	recStatus, err := sharing.RecStatus(db)
	if err != nil {
		return err
	}
	for _, rec := range recStatus {
		rec.Status = consts.PendingSharingStatus
	}

	sharing.Owner = true
	sharing.SharingID = utils.RandomString(32)

	return nil
}

// Create inserts a Sharing document in database
func Create(db couchdb.Database, doc *Sharing) error {
	err := couchdb.CreateDoc(db, doc)
	return err
}

var (
	_ couchdb.Doc    = &Sharing{}
	_ jsonapi.Object = &Sharing{}
)
