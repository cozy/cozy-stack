package workers

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

func init() {
	jobs.AddWorker("sharingupdates", &jobs.WorkerConfig{
		Concurrency:  4,
		MaxExecCount: 3,
		Timeout:      10 * time.Second,
		WorkerFunc:   SharingUpdates,
	})
}

var (
	// ErrSharingIDNotUnique is used when several occurences of the same sharing id are found
	ErrSharingIDNotUnique = errors.New("Several sharings with this id found")
	// ErrSharingDoesNotExist is used when the given sharing does not exist.
	ErrSharingDoesNotExist = errors.New("Sharing does not exist")
	// ErrDocumentNotLegitimate is used when a shared document is triggered but
	// not legitimate for this sharing
	ErrDocumentNotLegitimate = errors.New("Triggered illegitimate shared document")
	//ErrRecipientDoesNotExist is used when the given recipient does not exist
	ErrRecipientDoesNotExist = errors.New("Recipient with given ID does not exist")
	// ErrRecipientHasNoURL is used to signal that a recipient has no URL.
	ErrRecipientHasNoURL = errors.New("Recipient has no URL")
)

// TriggerEvent describes the fields retrieved after a triggered event
type TriggerEvent struct {
	Event   *EventDoc       `json:"event"`
	Message *SharingMessage `json:"message"`
}

// EventDoc describes the event returned by the trigger
type EventDoc struct {
	Type string `json:"type"`
	Doc  *couchdb.JSONDoc
}

// SharingMessage describes a sharing message
type SharingMessage struct {
	SharingID string `json:"sharing_id"`
	DocType   string `json:"doctype"`
}

// Sharing describes the sharing document structure
type Sharing struct {
	SharingType      string             `json:"sharing_type"`
	Permissions      permissions.Set    `json:"permissions,omitempty"`
	RecipientsStatus []*RecipientStatus `json:"recipients,omitempty"`
}

// RecipientStatus contains the information about a recipient for a sharing
type RecipientStatus struct {
	Status       string               `json:"status,omitempty"`
	RefRecipient couchdb.DocReference `json:"recipient,omitempty"`
	AccessToken  *auth.AccessToken
}

// SharingUpdates handles shared document updates
func SharingUpdates(ctx context.Context, m *jobs.Message) error {
	domain := ctx.Value(jobs.ContextDomainKey).(string)

	event := &TriggerEvent{}
	err := m.Unmarshal(&event)
	if err != nil {
		return err
	}
	sharingID := event.Message.SharingID
	docType := event.Message.DocType
	docID := event.Event.Doc.M["_id"].(string)

	// Get the sharing document
	db := couchdb.SimpleDatabasePrefix(domain)
	var res []Sharing
	err = couchdb.FindDocs(db, consts.Sharings, &couchdb.FindRequest{
		UseIndex: "by-sharing-id",
		Selector: mango.Equal("sharing_id", sharingID),
	}, &res)
	if err != nil {
		return err
	}
	if len(res) < 1 {
		return ErrSharingDoesNotExist
	} else if len(res) > 1 {
		return ErrSharingIDNotUnique
	}
	sharing := &res[0]

	// Check the updated document is legitimate for this sharing
	if err = checkDocument(sharing, docID); err != nil {
		return err
	}

	return sendToRecipients(db, domain, sharing, docType, docID)
}

// checkDocument checks the legitimity of the updated document to be shared
func checkDocument(sharing *Sharing, docID string) error {
	// Check sharing type
	if sharing.SharingType == consts.OneShotSharing {
		return ErrDocumentNotLegitimate
	}
	// Check permissions
	for _, rule := range sharing.Permissions {
		for _, val := range rule.Values {
			if val == docID {
				return nil
			}
		}
	}
	return ErrDocumentNotLegitimate
}

// sendToRecipients retreives the recipients and send the document
func sendToRecipients(db couchdb.Database, domain string, sharing *Sharing, docType, docID string) error {

	recInfos := make([]*RecipientInfo, len(sharing.RecipientsStatus))
	for i, rec := range sharing.RecipientsStatus {
		recDoc, err := GetRecipient(db, rec.RefRecipient.ID)
		if err != nil {
			return err
		}
		u, err := ExtractHost(recDoc.M["url"].(string))
		if err != nil {
			return err
		}
		info := &RecipientInfo{
			URL:   u,
			Token: rec.AccessToken.AccessToken,
		}
		recInfos[i] = info
	}
	opts := &SendOptions{
		DocID:      docID,
		DocType:    docType,
		Update:     true,
		Recipients: recInfos,
	}
	// TODO: handle file sharing
	if opts.DocType != consts.Files {
		return SendDoc(domain, opts)
	}
	return nil
}

// GetRecipient returns the Recipient stored in database from a given ID
func GetRecipient(db couchdb.Database, recID string) (*couchdb.JSONDoc, error) {
	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(db, consts.Recipients, recID, doc)
	if couchdb.IsNotFoundError(err) {
		err = ErrRecipientDoesNotExist
	}
	return doc, err
}

// ExtractHost returns the recipient's host, without the scheme
func ExtractHost(fullURL string) (string, error) {
	if fullURL == "" {
		return "", ErrRecipientHasNoURL
	}
	u, err := url.Parse(fullURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}
