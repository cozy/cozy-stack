package migrations

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/note"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/model/vfs/vfsswift"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/pkg/utils"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/ncw/swift"
)

const (
	swiftV1ToV2 = "swift-v1-to-v2"
	toSwiftV3   = "to-swift-v3"

	swiftV1ContainerPrefixCozy = "cozy-"
	swiftV1ContainerPrefixData = "data-"
	swiftV2ContainerPrefixCozy = "cozy-v2-"
	swiftV2ContainerPrefixData = "data-v2-"
	swiftV3ContainerPrefix     = "cozy-v3-"

	accountsToOrganization = "accounts-to-organization"
	notesMimeType          = "notes-mime-type"
)

// maxSimultaneousCalls is the maximal number of simultaneous calls to Swift
// made by a single migration.
const maxSimultaneousCalls = 8

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "migrations",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Reserved:     true,
		WorkerFunc:   worker,
		WorkerCommit: commit,
		Timeout:      6 * time.Hour,
	})
}

type message struct {
	Type string `json:"type"`
}

func worker(ctx *job.WorkerContext) error {
	var msg message
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}

	logger.WithDomain(ctx.Instance.Domain).WithField("nspace", "migration").
		Infof("Start the migration %s", msg.Type)

	switch msg.Type {
	case toSwiftV3:
		return migrateToSwiftV3(ctx.Instance.Domain)
	case swiftV1ToV2:
		return fmt.Errorf("this migration type is no longer supported")
	case accountsToOrganization:
		return migrateAccountsToOrganization(ctx.Instance.Domain)
	case notesMimeType:
		return migrateNotesMimeType(ctx.Instance.Domain)
	default:
		return fmt.Errorf("unknown migration type %q", msg.Type)
	}
}

func commit(ctx *job.WorkerContext, err error) error {
	log := logger.WithDomain(ctx.Instance.Domain).WithField("nspace", "migration")
	if err == nil {
		log.Infof("Migration success")
	} else {
		log.Errorf("Migration error: %s", err)
	}
	return err
}

func migrateNotesMimeType(domain string) error {
	inst, err := instance.GetFromCouch(domain)
	if err != nil {
		return err
	}
	log := inst.Logger().WithField("nspace", "migration")

	var docs []*vfs.FileDoc
	req := &couchdb.FindRequest{
		UseIndex: "by-mime-updated-at",
		Selector: mango.And(
			mango.Equal("mime", "text/markdown"),
			mango.Exists("updated_at"),
		),
		Limit: 1000,
	}
	_, err = couchdb.FindDocsRaw(inst, consts.Files, req, &docs)
	if err != nil {
		return err
	}
	log.Infof("Found %d markdown files", len(docs))
	for _, doc := range docs {
		if _, ok := doc.Metadata["version"]; !ok {
			log.Infof("Skip file %#v", doc)
			continue
		}
		if err := note.Update(inst, doc.ID()); err != nil {
			log.Warnf("Cannot change mime-type for note %s: %s", doc.ID(), err)
		}
	}

	return nil
}

// Migrate all the encrypted accounts to Bitwarden ciphers.
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
	type VaultReference struct {
		ID       string `json:"_id"`
		Type     string `json:"_type"`
		Protocol string `json:"_protocol"`
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
		var link string
		if manifest.VendorLink == nil {
			log.Warningf("No vendor_link in manifest for %s", msg.Slug)
			continue
		}
		if err := json.Unmarshal(*manifest.VendorLink, &link); err != nil {
			errm = multierror.Append(errm, err)
		}
		link = strings.Trim(link, "'")
		acc := &account.Account{}
		if err := couchdb.GetDoc(inst, consts.Accounts, msg.Account, acc); err != nil {
			errm = multierror.Append(errm, err)
			continue
		}
		encryptedCreds := acc.Basic.EncryptedCredentials
		login, password, err := account.DecryptCredentials(encryptedCreds)
		if err != nil {
			if err == account.ErrBadCredentials {
				log.Warningf("Bad credentials for account %s - %s", acc.ID(), acc.AccountType)
			} else {
				errm = multierror.Append(errm, err)
			}
			continue
		}
		// Special case if the email field is used instead of login
		if login == "" && acc.Basic.Email != "" {
			login = acc.Basic.Email
		}
		cipher, err := buildCipher(orgKey, msg.Slug, login, password, link)
		if err != nil {
			errm = multierror.Append(errm, err)
			continue
		}
		if err := couchdb.CreateDoc(inst, cipher); err != nil {
			errm = multierror.Append(errm, err)
		}
		// Add vault relationship
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

func buildCipher(orgKey []byte, slug, username, password, url string) (*bitwarden.Cipher, error) {
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

func migrateToSwiftV3(domain string) error {
	c := config.GetSwiftConnection()
	inst, err := instance.GetFromCouch(domain)
	if err != nil {
		return err
	}
	log := inst.Logger().WithField("nspace", "migration")

	var srcContainer, migratedFrom string
	// TODO(XXX): Use ContainerNames() instead of duplicating the container names logic here
	switch inst.SwiftLayout {
	case 0: // layout v1
		srcContainer = swiftV1ContainerPrefixCozy + inst.DBPrefix()
		migratedFrom = "v1"
	case 1: // layout v2
		srcContainer = swiftV2ContainerPrefixCozy + inst.DBPrefix()
		switch inst.DBPrefix() {
		case inst.Domain:
			migratedFrom = "v2a"
		case inst.Prefix:
			migratedFrom = "v2b"
		default:
			return instance.ErrInvalidSwiftLayout
		}
	case 2: // layout v3
		return nil // Nothing to do!
	default:
		return instance.ErrInvalidSwiftLayout
	}

	log.Infof("Migrating from swift layout %s to swift layout v3", migratedFrom)

	vfs := inst.VFS()
	root, err := vfs.DirByID(consts.RootDirID)
	if err != nil {
		return err
	}

	mutex := lock.LongOperation(inst, "vfs")
	if err = mutex.Lock(); err != nil {
		return err
	}
	defer mutex.Unlock()

	dstContainer := swiftV3ContainerPrefix + inst.DBPrefix()
	if _, _, err = c.Container(dstContainer); err != swift.ContainerNotFound {
		log.Errorf("Destination container %s already exists or something went wrong. Migration canceled.", dstContainer)
		return errors.New("Destination container busy")
	}
	if err = c.ContainerCreate(dstContainer, nil); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if err := vfsswift.DeleteContainer(c, dstContainer); err != nil {
				log.Errorf("Failed to delete v3 container %s: %s", dstContainer, err)
			}
		}
	}()

	if err = copyTheFilesToSwiftV3(inst, c, root, srcContainer, dstContainer); err != nil {
		return err
	}

	meta := &swift.Metadata{"cozy-migrated-from": migratedFrom}
	_ = c.ContainerUpdate(dstContainer, meta.ContainerHeaders())
	if in, err := instance.GetFromCouch(domain); err == nil {
		inst = in
	}
	inst.SwiftLayout = 2
	if err = couchdb.UpdateDoc(couchdb.GlobalDB, inst); err != nil {
		return err
	}

	// Migration done. Now clean-up oldies.

	// WARNING: Don't call `err` any error below in this function or the defer func
	//          will delete the new container even if the migration was successful

	if deleteErr := vfs.Delete(); deleteErr != nil {
		log.Errorf("Failed to delete old %s containers: %s", migratedFrom, deleteErr)
	}
	return nil
}

func copyTheFilesToSwiftV3(inst *instance.Instance, c *swift.Connection, root *vfs.DirDoc, src, dst string) error {
	nb := 0
	ch := make(chan error)
	log := logger.WithDomain(inst.Domain).
		WithField("nspace", "migration")

	var thumbsContainer string
	// TODO(XXX): Use ContainerNames() instead of duplicating the container names logic here
	switch inst.SwiftLayout {
	case 0: // layout v1
		thumbsContainer = swiftV1ContainerPrefixData + inst.Domain
	case 1: // layout v2
		thumbsContainer = swiftV2ContainerPrefixData + inst.DBPrefix()
	default:
		return instance.ErrInvalidSwiftLayout
	}

	// Use a system of tokens to limit the number of simultaneous calls to
	// Swift: only a goroutine that has a token can make a call.
	tokens := make(chan int, maxSimultaneousCalls)
	for k := 0; k < maxSimultaneousCalls; k++ {
		tokens <- k
	}

	fs := inst.VFS()
	errm := vfs.WalkAlreadyLocked(fs, root, func(_ string, d *vfs.DirDoc, f *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}
		if f == nil {
			return nil
		}

		srcName := getSrcName(inst, f)
		dstName := getDstName(inst, f)
		if srcName == "" || dstName == "" {
			return fmt.Errorf("Unexpected copy: %q -> %q", srcName, dstName)
		}

		nb++
		go func() {
			k := <-tokens
			err := utils.RetryWithExpBackoff(3, 200*time.Millisecond, func() error {
				_, err := c.ObjectCopy(src, srcName, dst, dstName, nil)
				return err
			})
			if err != nil {
				log.Warningf("Cannot copy file from %s %s to %s %s: %s",
					src, srcName, dst, dstName, err)
			}
			ch <- err
			tokens <- k
		}()

		// Copy the thumbnails
		if f.Class == "image" {
			srcSmall, srcMedium, srcLarge := getThumbsSrcNames(inst, f)
			dstSmall, dstMedium, dstLarge := getThumbsDstNames(inst, f)
			nb += 3
			go func() {
				k := <-tokens
				_, err := c.ObjectCopy(thumbsContainer, srcSmall, dst, dstSmall, nil)
				if err != nil {
					log.Infof("Cannot copy thumbnail small from %s %s to %s %s: %s",
						thumbsContainer, srcSmall, dst, dstSmall, err)
				}
				ch <- nil
				_, err = c.ObjectCopy(thumbsContainer, srcMedium, dst, dstMedium, nil)
				if err != nil {
					log.Infof("Cannot copy thumbnail medium from %s %s to %s %s: %s",
						thumbsContainer, srcMedium, dst, dstMedium, err)
				}
				ch <- nil
				_, err = c.ObjectCopy(thumbsContainer, srcLarge, dst, dstLarge, nil)
				if err != nil {
					log.Infof("Cannot copy thumbnail large from %s %s to %s %s: %s",
						thumbsContainer, srcLarge, dst, dstLarge, err)
				}
				ch <- nil
				tokens <- k
			}()
		}
		return nil
	})

	for i := 0; i < nb; i++ {
		if err := <-ch; err != nil {
			errm = multierror.Append(errm, err)
		}
	}
	// Get back the tokens to ensure that each goroutine can finish.
	for k := 0; k < maxSimultaneousCalls; k++ {
		<-tokens
	}
	return errm
}

func getSrcName(inst *instance.Instance, f *vfs.FileDoc) string {
	srcName := ""
	switch inst.SwiftLayout {
	case 0: // layout v1
		srcName = f.DirID + "/" + f.DocName
	case 1: // layout v2
		srcName = vfsswift.MakeObjectName(f.DocID)
	}
	return srcName
}

// XXX the f FileDoc can be modified to add an InternalID
func getDstName(inst *instance.Instance, f *vfs.FileDoc) string {
	if f.InternalID == "" {
		old := f.Clone().(*vfs.FileDoc)
		f.InternalID = vfsswift.NewInternalID()
		if err := couchdb.UpdateDocWithOld(inst, f, old); err != nil {
			return ""
		}
	}
	return vfsswift.MakeObjectNameV3(f.DocID, f.InternalID)
}

func getThumbsSrcNames(inst *instance.Instance, f *vfs.FileDoc) (string, string, string) {
	var small, medium, large string
	switch inst.SwiftLayout {
	case 0: // layout v1
		small = fmt.Sprintf("thumbs/%s-small", f.DocID)
		medium = fmt.Sprintf("thumbs/%s-medium", f.DocID)
		large = fmt.Sprintf("thumbs/%s-large", f.DocID)
	case 1: // layout v2
		obj := vfsswift.MakeObjectName(f.DocID)
		small = fmt.Sprintf("thumbs/%s-small", obj)
		medium = fmt.Sprintf("thumbs/%s-medium", obj)
		large = fmt.Sprintf("thumbs/%s-large", obj)
	}
	return small, medium, large
}

func getThumbsDstNames(inst *instance.Instance, f *vfs.FileDoc) (string, string, string) {
	obj := vfsswift.MakeObjectName(f.DocID)
	small := fmt.Sprintf("thumbs/%s-small", obj)
	medium := fmt.Sprintf("thumbs/%s-medium", obj)
	large := fmt.Sprintf("thumbs/%s-large", obj)
	return small, medium, large
}
