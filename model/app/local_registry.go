package app

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/utils"

	"github.com/spf13/afero"
)

var (
	ErrInvalidDirPath         = errors.New("invalid dir path")
	ErrInvalidIndexPath       = errors.New("invalid index path")
	ErrInvalidManifestPath    = errors.New("invalid manifest path")
	ErrUnexpectedResourceType = errors.New("unexpected resource type")
)

type LocalResource struct {
	Type consts.AppType
	Dir  string
}

// LocalRegistry is a map of slug -> LocalResource used during the development
// of webapps that are not installed in the Cozy but served from local
// directories.
type LocalRegistry struct {
	resources map[string]LocalResource
	fs        afero.Fs
}

// NewLocalRegistry instantiate a new [LocalResources].
func NewLocalRegistry(fs afero.Fs) *LocalRegistry {
	return &LocalRegistry{
		resources: map[string]LocalResource{},
		fs:        fs,
	}
}

// List return all the slugs
func (r *LocalRegistry) List() []string {
	res := []string{}

	for slug := range r.resources {
		res = append(res, slug)
	}

	return res
}

func (r *LocalRegistry) FileServer() appfs.FileServer {
	return appfs.NewAferoFileServer(r.fs, func(_, _, _, file string) string {
		return path.Join("/", file)
	})
}

func (r *LocalRegistry) Add(slug string, res LocalResource) error {
	absDir := utils.AbsPath(res.Dir)

	exists, err := utils.DirExistsFs(r.fs, absDir)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("%w: %w", ErrInvalidDirPath, os.ErrNotExist)
	}

	var manifestPath string
	var indexPath string
	switch res.Type {
	case consts.WebappType:
		manifestPath = path.Join(absDir, WebappManifestName)
		indexPath = path.Join(absDir, "index.html")
	case consts.KonnectorType:
		manifestPath = path.Join(absDir, KonnectorManifestName)
	default:
		return fmt.Errorf("%q: %w", res.Type, ErrUnexpectedResourceType)
	}

	err = fileExists(r.fs, manifestPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidManifestPath, err)
	}

	if indexPath != "" {
		err = fileExists(r.fs, indexPath)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidIndexPath, err)
		}
	}

	r.resources[slug] = res

	return nil
}

func (r *LocalRegistry) GetWebAppManifest(slug string) (*WebappManifest, error) {
	res, ok := r.resources[slug]
	if !ok {
		return nil, ErrNotFound
	}

	manFile, err := r.fs.Open(path.Join(res.Dir, WebappManifestName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("could not find the manifest in your app directory %s", res.Dir)
		}
		return nil, err
	}

	app := &WebappManifest{doc: &couchdb.JSONDoc{}}

	man, err := app.ReadManifest(manFile, slug, path.Join("file://localhost", res.Dir))
	if err != nil {
		return nil, fmt.Errorf("could not parse the manifest: %s", err.Error())
	}

	app = man.(*WebappManifest)
	app.FromLocalDir = true
	app.val.State = Ready

	return app, nil
}

func (r *LocalRegistry) GetKonnManifest(slug string) (*KonnManifest, error) {
	res, ok := r.resources[slug]
	if !ok {
		return nil, ErrNotFound
	}

	manFile, err := r.fs.Open(path.Join(res.Dir, KonnectorManifestName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("could not find the manifest in your konnector directory %s", res.Dir)
		}
		return nil, err
	}

	konn := &KonnManifest{doc: &couchdb.JSONDoc{}}

	man, err := konn.ReadManifest(manFile, slug, path.Join("file://localhost", res.Dir))
	if err != nil {
		return nil, fmt.Errorf("could not parse the manifest: %s", err.Error())
	}

	konn = man.(*KonnManifest)
	konn.FromLocalDir = true
	konn.val.State = Ready

	return konn, nil
}

func fileExists(fs afero.Fs, path string) error {
	exists, err := utils.FileExistsFs(fs, path)
	if err != nil {
		return err
	}

	if !exists {
		return os.ErrNotExist
	}

	return nil
}
