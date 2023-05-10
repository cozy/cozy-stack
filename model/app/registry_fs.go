package app

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/utils"

	"github.com/spf13/afero"
)

var (
	ErrInvalidDirPath         = errors.New("invalid dir path")
	ErrInvalidIndexPath       = errors.New("invalid index path")
	ErrInvalidManifestPath    = errors.New("invalid manifest path")
	ErrUnexpectedResourceType = errors.New("unexpected resource type")
)

var RegistryFS *LocalRegistry

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

// NewLocalRegistry instantiates a new [LocalResources].
func NewLocalRegistry(fs afero.Fs) *LocalRegistry {
	return &LocalRegistry{
		resources: map[string]LocalResource{},
		fs:        fs,
	}
}

// ListApps returns all the application's slugs handled by the registry.
func (r *LocalRegistry) ListApps() []string {
	res := []string{}

	for slug, resource := range r.resources {
		if resource.Type == consts.WebappType {
			res = append(res, slug)
		}
	}

	return res
}

// ListKonnss returns all the konnectors's slugs handled by the registry.
func (r *LocalRegistry) ListKonns() []string {
	res := []string{}

	for slug, resource := range r.resources {
		if resource.Type == consts.KonnectorType {
			res = append(res, slug)
		}
	}

	return res
}

// Contains returns true if an app with the given slug is stored inside
// this registry.
func (r *LocalRegistry) Contains(slug string) bool {
	_, ok := r.resources[slug]
	return ok
}

// FileServer returns an `appfs.FileServer` serving the app matching
// the given `slug`
func (r *LocalRegistry) FileServer(slug string) appfs.FileServer {
	resource := r.resources[slug]

	return appfs.NewAferoFileServer(r.fs, func(_, _, _, file string) string {
		return path.Join(resource.Dir, file)
	})
}

// Add an application described by the [LocalResource] into the registry.
func (r *LocalRegistry) Add(slug string, res LocalResource) error {
	absDir := utils.AbsPath(res.Dir)

	err := utils.DirExists(r.fs, absDir)
	if err != nil {
		return fmt.Errorf("invalide path %q: %w", absDir, err)
	}

	var manifestPath string
	var indexPath string

	switch res.Type {
	case consts.WebappType:
		manifestPath = path.Join(absDir, WebappManifestName)
		indexPath = path.Join(absDir, WebappIndexPath)

	case consts.KonnectorType:
		manifestPath = path.Join(absDir, KonnectorManifestName)

	default:
		return fmt.Errorf("%q: %w", res.Type, ErrUnexpectedResourceType)
	}

	err = utils.FileExists(r.fs, manifestPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidManifestPath, err)
	}

	if indexPath != "" {
		err = utils.FileExists(r.fs, indexPath)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidIndexPath, err)
		}
	}

	r.resources[slug] = res

	return nil
}

// GetWebAppManifest returns the manifest for the given slug.
//
// [ErrNotFound] is returned if no application matches the slug.
func (r *LocalRegistry) GetWebAppManifest(slug string) (*WebappManifest, error) {
	res, ok := r.resources[slug]
	if !ok {
		return nil, ErrNotFound
	}

	manFile, err := r.fs.Open(path.Join(res.Dir, WebappManifestName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to open the manifest at %q: %w", res.Dir, err)
		}
		return nil, err
	}

	man, err := NewWebAppManifestFromReader(manFile, slug, path.Join("file://localhost", res.Dir))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the manifest: %w", err)
	}

	man.SetState(Ready)

	return man, nil
}

// GetWebAppManifest returns the manifest for the given slug.
//
// [ErrNotFound] is returned if not konnected matches the slug.
func (r *LocalRegistry) GetKonnManifest(slug string) (*KonnManifest, error) {
	res, ok := r.resources[slug]
	if !ok {
		return nil, ErrNotFound
	}

	manFile, err := r.fs.Open(path.Join(res.Dir, KonnectorManifestName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to open the manifest at %q: %w", res.Dir, err)
		}
		return nil, err
	}

	man, err := NewKonnManifestFromReader(manFile, slug, path.Join("file://localhost", res.Dir))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the manifest: %w", err)
	}

	man.SetState(Ready)

	return man, nil
}
