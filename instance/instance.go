package instance

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/spf13/afero"
)

type dbPrefix struct{ prefix string }

func (p *dbPrefix) Prefix() string { return p.prefix + "/" }

var globalDBPrefix = &dbPrefix{"global"}

const instanceType = "instances"

var (
	// ErrNotFound is used when the seeked instance was not found
	ErrNotFound = errors.New("Instance not found")
	// ErrExists is used the instance already exists
	ErrExists = errors.New("Instance already exists")
	// ErrIllegalDomain is used when the domain named contains illegal characters
	ErrIllegalDomain = errors.New("Domain name contains illegal characters")
)

// An Instance has the informations relatives to the logical cozy instance,
// like the domain, the locale or the access to the databases and files storage
// It is a couchdb.Doc to be persisted in couchdb.
type Instance struct {
	DocID      string `json:"_id,omitempty"`  // couchdb _id
	DocRev     string `json:"_rev,omitempty"` // couchdb _rev
	Domain     string `json:"domain"`         // The main DNS domain, like example.cozycloud.cc
	StorageURL string `json:"storage"`        // Where the binaries are persisted
	storage    afero.Fs
}

// DocType implements couchdb.Doc
func (i *Instance) DocType() string { return instanceType }

// ID implements couchdb.Doc
func (i *Instance) ID() string { return i.DocID }

// SetID implements couchdb.Doc
func (i *Instance) SetID(v string) { i.DocID = v }

// Rev implements couchdb.Doc
func (i *Instance) Rev() string { return i.DocRev }

// SetRev implements couchdb.Doc
func (i *Instance) SetRev(v string) { i.DocRev = v }

// ensure Instance implements couchdb.Doc & vfs.Context
var _ couchdb.Doc = (*Instance)(nil)
var _ vfs.Context = (*Instance)(nil)

// CreateInCouchdb create the instance doc in the global database
func (i *Instance) createInCouchdb() (err error) {
	if _, err = Get(i.Domain); err == nil {
		return ErrExists
	}
	if err != nil && err != ErrNotFound {
		return err
	}
	err = couchdb.CreateDoc(globalDBPrefix, i)
	if err != nil {
		return err
	}
	byDomain := mango.IndexOnFields("domain")
	return couchdb.DefineIndex(globalDBPrefix, instanceType, byDomain)
}

// createRootFolder creates the root folder for this instance
func (i *Instance) createRootFolder() error {
	vfsC, err := i.GetVFSContext()
	if err != nil {
		return err
	}

	rootFsURL := config.BuildAbsFsURL("/")
	domainURL := config.BuildRelFsURL(i.Domain)

	rootFs, err := createFs(rootFsURL)
	if err != nil {
		return err
	}

	if err = rootFs.MkdirAll(domainURL.Path, 0755); err != nil {
		return err
	}

	if err = vfs.CreateRootDirDoc(vfsC); err != nil {
		rootFs.Remove(domainURL.Path)
		return err
	}

	return nil
}

// createFSIndexes creates the index needed by VFS
func (i *Instance) createFSIndexes() error {
	for _, index := range vfs.Indexes {
		err := couchdb.DefineIndex(i, vfs.FsDocType, index)
		if err != nil {
			return err
		}
	}
	return nil
}

// Create build an instance and .Create it
func Create(domain string, locale string, apps []string) (*Instance, error) {
	if strings.ContainsAny(domain, vfs.ForbiddenFilenameChars) || domain == ".." || domain == "." {
		return nil, ErrIllegalDomain
	}

	domainURL := config.BuildRelFsURL(domain)
	i := &Instance{
		Domain:     domain,
		StorageURL: domainURL.String(),
	}

	err := i.Create()
	if err != nil {
		return nil, err
	}

	return i, nil
}

func (i *Instance) checkAndMakeStorage() error {
	u, err := url.Parse(i.StorageURL)
	if err != nil {
		return err
	}
	switch u.Scheme {
	case "file":
		i.storage = afero.NewBasePathFs(afero.NewOsFs(), u.Path)
	case "mem":
		i.storage = afero.NewMemMapFs()
	default:
		return fmt.Errorf("Unknown storage provider: %v", u.Scheme)
	}
	return nil
}

// Create performs the necessary setups for this instance to be usable
func (i *Instance) Create() error {
	if err := i.createInCouchdb(); err != nil {
		return err
	}

	if err := i.createRootFolder(); err != nil {
		return err
	}

	if err := i.createFSIndexes(); err != nil {
		return err
	}
	// TODO atomicity with defer
	// TODO figure out what to do with locale
	// TODO install apps

	return nil
}

// Get retrieves the instance for a request by its host.
func Get(domain string) (*Instance, error) {
	if config.IsDevRelease() {
		if domain == "" || strings.Contains(domain, "127.0.0.1") || strings.Contains(domain, "localhost") {
			domain = "dev"
		}
	}

	var instances []*Instance
	req := &couchdb.FindRequest{
		Selector: mango.Equal("domain", domain),
		Limit:    1,
	}
	err := couchdb.FindDocs(globalDBPrefix, instanceType, req, &instances)
	if couchdb.IsNoDatabaseError(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, ErrNotFound
	}

	if err = instances[0].checkAndMakeStorage(); err != nil {
		return nil, err
	}

	return instances[0], nil
}

// List returns the list of declared instances.
//
// TODO: pagination
// TODO: don't return the design docs
func List() ([]*Instance, error) {
	var docs []*Instance
	sel := mango.Empty()
	req := &couchdb.FindRequest{Selector: sel, Limit: 100}
	err := couchdb.FindDocs(globalDBPrefix, instanceType, req, &docs)
	return docs, err
}

// Destroy is used to remove the instance. All the data linked to this
// instance will be permanently deleted.
func Destroy(domain string) (*Instance, error) {
	i, err := Get(domain)
	if err != nil {
		return nil, err
	}

	if err = couchdb.DeleteDoc(globalDBPrefix, i); err != nil {
		return nil, err
	}

	if err = couchdb.DeleteAllDBs(i); err != nil {
		return nil, err
	}

	rootFsURL := config.BuildAbsFsURL("/")
	domainURL := config.BuildRelFsURL(i.Domain)

	rootFs, err := createFs(rootFsURL)
	if err != nil {
		return nil, err
	}

	if err = rootFs.RemoveAll(domainURL.Path); err != nil {
		return nil, err
	}

	return i, nil
}

// FS returns the afero storage provider where the binaries for
// the current instance are persisted
func (i *Instance) FS() afero.Fs {
	if i.storage == nil {
		storageURL, err := url.Parse(i.StorageURL)
		if err != nil {
			panic(err)
		}
		i.storage, err = createFs(storageURL)
		if err != nil {
			panic(err)
		}
	}
	return i.storage
}

// Prefix returns the prefix to use in database naming for the
// current instance
func (i *Instance) Prefix() string {
	return i.Domain + "/"
}

func createFs(u *url.URL) (fs afero.Fs, err error) {
	switch u.Scheme {
	case "file":
		fs = afero.NewBasePathFs(afero.NewOsFs(), u.Path)
	case "mem":
		fs = afero.NewMemMapFs()
	default:
		err = fmt.Errorf("Unknown storage provider: %v", u.Scheme)
	}
	return
}
