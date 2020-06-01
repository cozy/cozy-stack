package migrations

import (
	"encoding/json"
	"strings"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/sirupsen/logrus"

	multierror "github.com/hashicorp/go-multierror"
)

type VaultReference struct {
	ID       string `json:"_id"`
	Type     string `json:"_type"`
	Protocol string `json:"_protocol"`
}

func isAdditionalField(fieldName string) bool {
	return !(fieldName == "login" ||
		fieldName == "password" ||
		fieldName == "advancedFields")
}

// Builds a cipher from an io.cozy.account
//
// A raw JSONDoc is used to be able to access auth.fields
func buildCipher(orgKey []byte, manifest *app.KonnManifest, account couchdb.JSONDoc, url string, logger *logrus.Entry) (*bitwarden.Cipher, error) {
	logger.Infof("Building ciphers...")

	auth, _ := account.M["auth"].(map[string]interface{})

	username, _ := auth["login"].(string)
	password, _ := auth["password"].(string)
	email, _ := auth["email"].(string)

	// Special case if the email field is used instead of login
	if username == "" && email != "" {
		username = email
	}

	key := orgKey[:32]
	hmac := orgKey[32:]

	ivURL := crypto.GenerateRandomBytes(16)
	encURL, err := crypto.EncryptWithAES256HMAC(key, hmac, []byte(url), ivURL)
	if err != nil {
		return nil, err
	}
	u := bitwarden.LoginURI{URI: encURL, Match: nil}
	uris := []bitwarden.LoginURI{u}

	ivName := crypto.GenerateRandomBytes(16)
	encName, err := crypto.EncryptWithAES256HMAC(key, hmac, []byte(manifest.Name), ivName)
	if err != nil {
		return nil, err
	}

	ivUsername := crypto.GenerateRandomBytes(16)
	encUsername, err := crypto.EncryptWithAES256HMAC(key, hmac, []byte(username), ivUsername)
	if err != nil {
		return nil, err
	}

	ivPassword := crypto.GenerateRandomBytes(16)
	encPassword, err := crypto.EncryptWithAES256HMAC(key, hmac, []byte(password), ivPassword)
	if err != nil {
		return nil, err
	}

	login := &bitwarden.LoginData{
		Username: encUsername,
		Password: encPassword,
		URIs:     uris,
	}

	md := metadata.New()
	md.DocTypeVersion = bitwarden.DocTypeVersion

	bitwardenFields := make([]bitwarden.Field, 0)

	for name, rawValue := range auth {
		value, ok := rawValue.(string)
		if !ok {
			continue
		}
		if !isAdditionalField(name) {
			continue
		}

		ivName := crypto.GenerateRandomBytes(16)
		encName, err := crypto.EncryptWithAES256HMAC(key, hmac, []byte(name), ivName)
		if err != nil {
			return nil, err
		}

		ivValue := crypto.GenerateRandomBytes(16)
		encValue, err := crypto.EncryptWithAES256HMAC(key, hmac, []byte(value), ivValue)
		if err != nil {
			return nil, err
		}

		logger.Infof("Adding field %s = %s", name, value)
		field := bitwarden.Field{
			Name:  encName,
			Value: encValue,
			Type:  bitwarden.FieldTypeText,
		}
		bitwardenFields = append(bitwardenFields, field)
	}

	c := bitwarden.Cipher{
		Type:           bitwarden.LoginType,
		Name:           encName,
		Login:          login,
		SharedWithCozy: true,
		Metadata:       md,
		Fields:         bitwardenFields,
	}
	return &c, nil
}

func getCipherLinkFromManifest(manifest *app.KonnManifest) (string, error) {
	var link string
	if manifest.VendorLink == nil {
		return "", nil
	}
	if err := json.Unmarshal(*manifest.VendorLink, &link); err != nil {
		return "", err
	}
	link = strings.Trim(link, "'")
	return link, nil
}

func updateSettings(inst *instance.Instance, attempt int, logger *logrus.Entry) error {
	logger.Infof("Updating bitwarden settings after migration...")
	// Reload the setting in case the revision changed
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	// This flag is checked at the extension pre-login to run the migration or not
	setting.ExtensionInstalled = true
	err = settings.UpdateRevisionDate(inst, setting)
	if err != nil {
		if couchdb.IsConflictError(err) && attempt < 2 {
			return updateSettings(inst, attempt+1, logger)
		}
	}
	return nil
}

func addCipherRelationshipToAccount(acc couchdb.JSONDoc, cipher *bitwarden.Cipher) {
	vRef := VaultReference{
		ID:       cipher.ID(),
		Type:     consts.BitwardenCiphers,
		Protocol: consts.BitwardenProtocol,
	}

	relationships, ok := acc.M["relationships"].(map[string]interface{})
	if !ok {
		relationships = make(map[string]interface{})
	}

	rel := make(map[string][]VaultReference)
	rel["data"] = []VaultReference{vRef}

	relationships[consts.BitwardenCipherRelationship] = rel

	acc.M["relationships"] = relationships
}

// Migrates all the encrypted accounts to Bitwarden ciphers.
// It decrypts each account, reencrypt the fields with the organization key,
// and save it in the ciphers database.
func migrateAccountsToOrganization(domain string) error {
	inst, err := instance.GetFromCouch(domain)
	if err != nil {
		return err
	}
	log := inst.Logger().WithField("nspace", "migration")

	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	// Get org key
	if err := setting.EnsureCozyOrganization(inst); err != nil {
		return err
	}
	orgKey, err := setting.OrganizationKey()
	if err != nil {
		return err
	}

	// Iterate over all triggers to get the konnectors with the associated account
	jobsSystem := job.System()
	triggers, err := jobsSystem.GetAllTriggers(inst)
	if err != nil {
		return err
	}
	var msg struct {
		Account string `json:"account"`
		Slug    string `json:"konnector"`
	}

	var errm error
	for _, t := range triggers {
		if t.Infos().WorkerType != "konnector" {
			continue
		}
		err := t.Infos().Message.Unmarshal(&msg)
		if err != nil || msg.Account == "" || msg.Slug == "" {
			continue
		}

		manifest, err := app.GetKonnectorBySlug(inst, msg.Slug)
		if err != nil {
			log.Warningf("Could not get manifest for %s", msg.Slug)
			continue
		}

		link, err := getCipherLinkFromManifest(manifest)
		if err != nil {
			errm = multierror.Append(errm, err)
			continue
		}

		if link == "" {
			log.Warningf("No vendor_link in manifest for %s", msg.Slug)
			continue
		}

		var accJson couchdb.JSONDoc

		if err := couchdb.GetDoc(inst, consts.Accounts, msg.Account, &accJson); err != nil {
			errm = multierror.Append(errm, err)
			continue
		}

		accJson.Type = consts.Accounts

		data.DecryptAccount(accJson)

		cipher, err := buildCipher(orgKey, manifest, accJson, link, log)
		if err != nil {
			errm = multierror.Append(errm, err)
			continue
		}
		if err := couchdb.CreateDoc(inst, cipher); err != nil {
			errm = multierror.Append(errm, err)
			continue
		}

		addCipherRelationshipToAccount(accJson, cipher)

		data.EncryptAccount(accJson)

		log.Infof("Updating doc %s", accJson)
		if err := couchdb.UpdateDoc(inst, &accJson); err != nil {
			errm = multierror.Append(errm, err)
			continue
		}
	}

	err = updateSettings(inst, 0, log)
	if err != nil {
		errm = multierror.Append(errm, err)
	}
	return errm
}
