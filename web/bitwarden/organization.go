package bitwarden

import (
	"errors"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/crypto"
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

func getCozyOrganizationResponse(inst *instance.Instance, settings *settings.Settings) (*organizationResponse, error) {
	if settings == nil || settings.PublicKey == "" {
		return nil, errors.New("No public key")
	}
	orgKey, err := settings.OrganizationKey()
	if err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Cannot read the organization key: %s", err)
		return nil, err
	}
	key, err := crypto.EncryptWithRSA(settings.PublicKey, orgKey)
	if err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Cannot encrypt with RSA: %s", err)
		return nil, err
	}

	email := inst.PassphraseSalt()
	return &organizationResponse{
		ID:             settings.OrganizationID,
		Name:           consts.BitwardenCozyOrganizationName,
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

func getCozyCollectionResponse(settings *settings.Settings) (*collectionResponse, error) {
	orgKey, err := settings.OrganizationKey()
	if err != nil || len(orgKey) != 64 {
		return nil, errors.New("Missing organization key")
	}
	iv := crypto.GenerateRandomBytes(16)
	payload := []byte(consts.BitwardenCozyCollectionName)
	name, err := crypto.EncryptWithAES256HMAC(orgKey[:32], orgKey[32:], payload, iv)
	if err != nil {
		return nil, err
	}
	return &collectionResponse{
		ID:             settings.CollectionID,
		OrganizationID: settings.OrganizationID,
		Name:           name,
		Object:         "collection",
	}, nil
}
