package migrations

import (
    "encoding/json"
    "strings"

    "github.com/cozy/cozy-stack/model/account"
    "github.com/cozy/cozy-stack/model/app"
    "github.com/cozy/cozy-stack/model/bitwarden"
    "github.com/cozy/cozy-stack/model/bitwarden/settings"
    "github.com/cozy/cozy-stack/model/instance"
    "github.com/cozy/cozy-stack/model/job"
    "github.com/cozy/cozy-stack/pkg/consts"
    "github.com/cozy/cozy-stack/pkg/couchdb"
    "github.com/cozy/cozy-stack/pkg/crypto"
    "github.com/cozy/cozy-stack/pkg/metadata"
    "github.com/sirupsen/logrus"

    multierror "github.com/hashicorp/go-multierror"
)


type VaultReference struct {
    ID       string `json:"_id"`
    Type     string `json:"_type"`
    Protocol string `json:"_protocol"`
}

func buildCipher(orgKey []byte, slug string, acc *account.Account, url string, logger *logrus.Entry) (*bitwarden.Cipher, error) {

    encryptedCreds := acc.Basic.EncryptedCredentials
    username, password, err := account.DecryptCredentials(encryptedCreds)
    if err != nil {
        return nil, err
    }
    // Special case if the email field is used instead of login
    if username == "" && acc.Basic.Email != "" {
        username = acc.Basic.Email
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
    encName, err := crypto.EncryptWithAES256HMAC(key, hmac, []byte(slug), ivName)
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

    c := bitwarden.Cipher{
        Type:           bitwarden.LoginType,
        Name:           encName,
        Login:          login,
        SharedWithCozy: true,
        Metadata:       md,
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

func linkAccountToCipher(acc * account.Account, cipher * bitwarden.Cipher) {
    vRef := VaultReference{
        ID:       cipher.ID(),
        Type:     consts.BitwardenCiphers,
        Protocol: consts.BitwardenProtocol,
    }

    if acc.Relationships == nil {
        acc.Relationships = make(map[string]interface{})
    }

    rel := make(map[string][]VaultReference)
    rel["data"] = []VaultReference{vRef}
    acc.Relationships[consts.BitwardenCipherRelationship] = rel
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

        if (err != nil) {
            errm = multierror.Append(errm, err)
            continue
        }

        if link != "" {
            log.Warningf("No vendor_link in manifest for %s", msg.Slug)
            continue
        }

        acc := &account.Account{}
        if err := couchdb.GetDoc(inst, consts.Accounts, msg.Account, acc); err != nil {
            errm = multierror.Append(errm, err)
            continue
        }

        cipher, err := buildCipher(orgKey, msg.Slug, acc, link, log)
        if err != nil {
            if err == account.ErrBadCredentials {
                log.Warningf("Bad credentials for account %s - %s", acc.ID(), acc.AccountType)
            } else {
                errm = multierror.Append(errm, err)
            }
            continue
        }
        if err := couchdb.CreateDoc(inst, cipher); err != nil {
            errm = multierror.Append(errm, err)
        }

        linkAccountToCipher(acc, cipher)

        if err := couchdb.UpdateDoc(inst, acc); err != nil {
            errm = multierror.Append(errm, err)
        }
    }
    // Reload the setting in case the revision changed
    setting, err = settings.Get(inst)
    if err != nil {
        errm = multierror.Append(errm, err)
        return errm
    }
    // This flag is checked at the extension pre-login to run the migration or not
    setting.ExtensionInstalled = true
    err = settings.UpdateRevisionDate(inst, setting)
    if err != nil {
        if !couchdb.IsConflictError(err) {
            errm = multierror.Append(errm, err)
            return errm
        }
        // The settings have been updated elsewhere: retry
        setting, err = settings.Get(inst)
        if err != nil {
            errm = multierror.Append(errm, err)
            return errm
        }
        setting.ExtensionInstalled = true
        err = settings.UpdateRevisionDate(inst, setting)
        if err != nil {
            errm = multierror.Append(errm, err)
        }
    }
    return errm
}
