package sharings

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
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
	Sharer           *Sharer            `json:"sharer,omitempty"`
}

// RecipientStatus contains the information about a recipient for a sharing
type RecipientStatus struct {
	Status       string `json:"status,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`

	RefRecipient jsonapi.ResourceIdentifier `json:"recipient,omitempty"`

	recipient *Recipient
}

// Sharer contains the information about the sharer from the recipient's
// perspective.
//
// ATTENTION: This structure will only be filled by the recipients as it is
// recipient specific. The `ClientID` is different for each recipient and only
// known by them.
type Sharer struct {
	ClientID string `json:"client_id"`
	URL      string `json:"url"`
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

		if recipient.Client == nil {
			return nil, nil
		}
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
		UseIndex: "by-sharing-id",
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

// addTrigger creates a new trigger on the updates of the shared documents
func addTrigger(instance *instance.Instance, rule permissions.Rule, sharingID string) error {
	scheduler := instance.JobsScheduler()

	eventArgs := rule.Type + ":UPDATED:" + strings.Join(rule.Values, ",")
	type TriggerMessage struct {
		SharingID string `json:"sharing_id"`
	}
	m := TriggerMessage{sharingID}
	workerArgs, err := json.Marshal(m)
	if err != nil {
		return err
	}
	t, err := jobs.NewTrigger(&jobs.TriggerInfos{
		Type:       "@event",
		WorkerType: "sharing",
		Arguments:  eventArgs,
		Message: &jobs.Message{
			Type: jobs.JSONEncoding,
			Data: workerArgs,
		},
	})
	if err != nil {
		return err
	}
	return scheduler.Add(t)
}

// sendDoc sends a JSON document to a recipient
func sendDoc(docType, id string, doc *couchdb.JSONDoc, recStatus *RecipientStatus) error {
	// Get the recipient info
	token := recStatus.AccessToken
	rec := recStatus.recipient
	domain, err := rec.ExtractDomain()
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/data/%s/%s", docType, id)
	// A new doc will be created on the recipient side
	delete(doc.M, "_id")
	delete(doc.M, "_rev")
	body, err := request.WriteJSON(doc.M)
	if err != nil {
		return err
	}

	_, err = request.Req(&request.Options{
		Domain: domain,
		Method: "PUT",
		Path:   path,
		Headers: request.Headers{
			"Content-Type":  "application/json",
			"Accept":        "application/json",
			"Authorization": "Bearer " + token,
		},
		Body: body,
	})
	return err
}

// ShareDoc shares the documents specified in the Sharing structure to the
// specified recipient
func ShareDoc(instance *instance.Instance, sharing *Sharing, recStatus *RecipientStatus) error {
	// Lookup all the sharing permissions
	for _, rule := range sharing.Permissions {
		// Only static values are supported yet
		if len(rule.Values) == 0 {
			return nil
		}
		docType := rule.Type
		// Create a trigger for the updates on the rule
		if err := addTrigger(instance, rule, sharing.SharingID); err != nil {
			return err
		}
		// Get each document referenced in Values and sent it to the recipient
		for _, val := range rule.Values {
			doc := &couchdb.JSONDoc{}
			if err := couchdb.GetDoc(instance, docType, val, doc); err != nil {
				return err
			}
			if err := sendDoc(docType, val, doc, recStatus); err != nil {
				return err
			}

		}
	}
	return nil
}

// SharingAccepted handles an accepted sharing on the sharer side
func SharingAccepted(instance *instance.Instance, state, clientID, accessCode string) (string, error) {
	sharing, recStatus, err := findSharingRecipient(instance, state, clientID)
	if err != nil {
		return "", err
	}
	recStatus.Status = consts.AcceptedSharingStatus

	access, err := recStatus.recipient.GetAccessToken(accessCode)
	if err != nil {
		return "", err
	}
	recStatus.AccessToken = access.AccessToken
	recStatus.RefreshToken = access.RefreshToken
	err = couchdb.UpdateDoc(instance, sharing)
	if err != nil {
		return "", err
	}
	// Share all the documents with the recipient
	err = ShareDoc(instance, sharing, recStatus)

	// Redirect the recipient after acceptation
	redirect := recStatus.recipient.URL
	return redirect, err
}

// SharingRefused handles a rejected sharing on the sharer side
func SharingRefused(db couchdb.Database, state, clientID string) (string, error) {
	sharing, recStatus, err := findSharingRecipient(db, state, clientID)
	if err != nil {
		return "", err
	}
	recStatus.Status = consts.RefusedSharingStatus
	err = couchdb.UpdateDoc(db, sharing)
	redirect := recStatus.recipient.URL
	return redirect, err
}

// RecipientRefusedSharing executes all the actions induced by a refusal from
// the recipient: the sharing document is deleted and the sharer is informed.
func RecipientRefusedSharing(db couchdb.Database, sharingID, clientID string) (string, error) {
	// We get the sharing document through its sharing id…
	var res []Sharing
	err := couchdb.FindDocs(db, consts.Sharings, &couchdb.FindRequest{
		Selector: mango.Equal("sharing_id", sharingID),
	}, &res)
	if err != nil {
		return "", err
	} else if len(res) < 1 {
		return "", ErrSharingDoesNotExist
	} else if len(res) > 1 {
		return "", ErrSharingIDNotUnique
	}
	sharing := &res[0]

	// … and we delete it because it is no longer needed.
	err = couchdb.DeleteDoc(db, sharing)
	if err != nil {
		return "", err
	}
	// We get the sharer's oauth client so that we can get her Cozy's url
	// through the `ClientURI`.
	sharer := &oauth.Client{}
	err = couchdb.GetDoc(db, consts.OAuthClients, clientID, sharer)
	if err != nil {
		return "", ErrNoOAuthClient
	}

	u := fmt.Sprintf("%s/sharings/answer", sharer.ClientURI)
	return u, nil
}

// CreateSharingRequest checks fields integrity and creates a sharing document
// for an incoming sharing request
func CreateSharingRequest(db couchdb.Database, desc, state, sharingType, scope, clientID string) (*Sharing, error) {
	if state == "" {
		return nil, ErrMissingState
	}
	if err := CheckSharingType(sharingType); err != nil {
		return nil, err
	}
	if scope == "" {
		return nil, ErrMissingScope
	}
	if clientID == "" {
		return nil, ErrNoOAuthClient
	}
	permissions, err := permissions.UnmarshalScopeString(scope)
	if err != nil {
		return nil, err
	}

	sharerClient := &oauth.Client{}
	err = couchdb.GetDoc(db, consts.OAuthClients, clientID, sharerClient)
	if err != nil {
		return nil, ErrNoOAuthClient
	}

	sharer := &Sharer{
		ClientID: clientID,
		URL:      sharerClient.ClientURI,
	}

	sharing := &Sharing{
		SharingType: sharingType,
		SharingID:   state,
		Permissions: permissions,
		Owner:       false,
		Desc:        desc,
		Sharer:      sharer,
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
