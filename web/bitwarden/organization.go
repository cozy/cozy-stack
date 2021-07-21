package bitwarden

import (
	"errors"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/crypto"
)

// https://github.com/bitwarden/jslib/blob/master/common/src/models/response/profileOrganizationResponse.ts
type organizationResponse struct {
	ID             string  `json:"Id"`
	Identifier     *string `json:"Identifier"`
	Name           string  `json:"Name"`
	Key            string  `json:"Key"`
	Email          string  `json:"BillingEmail"`
	Plan           string  `json:"Plan"`
	PlanType       int     `json:"PlanType"`
	Seats          int     `json:"Seats"`
	MaxCollections int     `json:"MaxCollections"`
	MaxStorage     int     `json:"MaxStorageGb"`
	SelfHost       bool    `json:"SelfHost"`
	Use2fa         bool    `json:"Use2fa"`
	UseDirectory   bool    `json:"UseDirectory"`
	UseEvents      bool    `json:"UseEvents"`
	UseGroups      bool    `json:"UseGroups"`
	UseTotp        bool    `json:"UseTotp"`
	UseAPI         bool    `json:"UseApi"`
	UsePolicies    bool    `json:"UsePolicies"`
	UseSSO         bool    `json:"UseSSO"`
	UseResetPass   bool    `json:"UseResetPassword"`
	HasKeys        bool    `json:"HasPublicAndPrivateKeys"`
	ResetPass      bool    `json:"ResetPasswordEnrolled"`
	Premium        bool    `json:"UsersGetPremium"`
	Enabled        bool    `json:"Enabled"`
	Status         int     `json:"Status"`
	Type           int     `json:"Type"`
	Object         string  `json:"Object"`
}

func newOrganizationResponse(inst *instance.Instance, org *bitwarden.Organization) *organizationResponse {
	m := org.Members[inst.Domain]
	return &organizationResponse{
		ID:             org.ID(),
		Identifier:     nil, // Not supported by us
		Name:           org.Name,
		Key:            m.Key,
		Email:          m.Email,
		Plan:           "TeamsAnnually",
		PlanType:       9,  // TeamsAnnually plan
		Seats:          10, // The value doesn't matter
		MaxCollections: 1,
		MaxStorage:     1,
		SelfHost:       true,
		Use2fa:         true,
		UseDirectory:   false,
		UseEvents:      false,
		UseGroups:      false,
		UseTotp:        true,
		UseAPI:         false,
		UsePolicies:    false,
		UseSSO:         false,
		UseResetPass:   false,
		HasKeys:        false, // The public/private keys are used for the Admin Reset Password feature, not implemented by us
		ResetPass:      false,
		Premium:        true,
		Enabled:        true,
		Status:         int(m.Status),
		Type:           2, // User
		Object:         "profileOrganization",
	}
}

func getCozyOrganizationResponse(inst *instance.Instance, setting *settings.Settings) (*organizationResponse, error) {
	org, err := bitwarden.GetCozyOrganization(inst, setting)
	if err != nil {
		return nil, err
	}
	return newOrganizationResponse(inst, org), nil
}

// https://github.com/bitwarden/jslib/blob/master/common/src/models/response/collectionResponse.ts
type collectionResponse struct {
	ID             string `json:"Id"`
	OrganizationID string `json:"OrganizationId"`
	Name           string `json:"Name"`
	Object         string `json:"Object"`
}

func getCozyCollectionResponse(setting *settings.Settings) (*collectionResponse, error) {
	orgKey, err := setting.OrganizationKey()
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
		ID:             setting.CollectionID,
		OrganizationID: setting.OrganizationID,
		Name:           name,
		Object:         "collection",
	}, nil
}
