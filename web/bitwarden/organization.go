package bitwarden

import (
	"errors"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/crypto"
)

const (
	// https://xkcd.com/221/
	cozyOrganizationID   = "38ac39d0-d48d-11e9-91bf-f37e45d48c79"
	cozyOrganizationName = "Cozy"
	cozyCollectionID     = "385aaa2a-d48d-11e9-bb5f-6b31dfebcb4d"
	cozyCollectionName   = "Cozy Connectors"
)

// https://github.com/bitwarden/jslib/blob/master/src/models/response/profileOrganizationResponse.ts
type organizationResponse struct {
	ID             string `json:"Id"`
	Name           string `json:"Name"`
	Key            string `json:"Key"`
	Email          string `json:"BillingEmail"`
	Plan           string `json:"Plan"`
	PlanType       int    `json:"PlanType"`
	Seats          int    `json:"Seats"`
	MaxCollections int    `json:"MaxCollections"`
	MaxStorage     int    `json:"MaxStorageGb"`
	SelfHost       bool   `json:"SelfHost"`
	Use2fa         bool   `json:"Use2fa"`
	UseDirectory   bool   `json:"UseDirectory"`
	UseEvents      bool   `json:"UseEvents"`
	UseGroups      bool   `json:"UseGroups"`
	UseTotp        bool   `json:"UseTotp"`
	Premium        bool   `json:"UsersGetPremium"`
	Enabled        bool   `json:"Enabled"`
	Status         int    `json:"Status"`
	Type           int    `json:"Type"`
	Object         string `json:"Object"`
}

func getOrgKey() []byte {
	key := make([]byte, 64)
	for i := range key {
		key[i] = 'x'
	}
	return key
}

func getCozyOrganizationResponse(inst *instance.Instance, settings *settings.Settings) (*organizationResponse, error) {
	if settings == nil || settings.PublicKey == "" {
		return nil, errors.New("No public key")
	}
	orgKey := getOrgKey()
	key, err := crypto.EncryptWithRSA(settings.PublicKey, orgKey)
	if err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Cannot encrypt with RSA: %s", err)
		return nil, err
	}

	email := inst.PassphraseSalt()
	return &organizationResponse{
		ID:             cozyOrganizationID,
		Name:           cozyOrganizationName,
		Key:            key,
		Email:          string(email),
		Plan:           "TeamsAnnually",
		PlanType:       5, // TeamsAnnually plan
		Seats:          2,
		MaxCollections: 1,
		MaxStorage:     1,
		SelfHost:       true,
		Use2fa:         true,
		UseDirectory:   false,
		UseEvents:      false,
		UseGroups:      false,
		UseTotp:        true,
		Premium:        true,
		Enabled:        true,
		Status:         2, // Confirmed
		Type:           2, // User
		Object:         "profileOrganization",
	}, nil
}

// https://github.com/bitwarden/jslib/blob/master/src/models/response/collectionResponse.ts
type collectionResponse struct {
	ID             string `json:"Id"`
	OrganizationID string `json:"OrganizationId"`
	Name           string `json:"Name"`
	Object         string `json:"Object"`
}

func getCozyCollectionResponse() (*collectionResponse, error) {
	orgKey := getOrgKey()
	iv := crypto.GenerateRandomBytes(16)
	payload := []byte(cozyCollectionName)
	name, err := crypto.EncryptWithAES256HMAC(orgKey[:32], orgKey[32:], payload, iv)
	if err != nil {
		return nil, err
	}
	return &collectionResponse{
		ID:             cozyCollectionID,
		OrganizationID: cozyOrganizationID,
		Name:           name,
		Object:         "collection",
	}, nil
}
