package lifecycle

import (
	"fmt"
	"path"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/model/vfs/vfsafero"
	"github.com/cozy/cozy-stack/model/vfs/vfsswift"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/spf13/afero"
)

// ThumbsFS returns the hidden filesystem for storing the thumbnails of the
// photos/image
func ThumbsFS(i *instance.Instance) vfs.Thumbser {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile:
		baseFS := afero.NewBasePathFs(afero.NewOsFs(),
			path.Join(fsURL.Path, i.DirName(), vfs.ThumbsDirName))
		return vfsafero.NewThumbsFs(baseFS)
	case config.SchemeMem:
		baseFS := vfsafero.GetMemFS(i.DomainName() + "-thumbs")
		return vfsafero.NewThumbsFs(baseFS)
	case config.SchemeSwift, config.SchemeSwiftSecure:
		switch i.SwiftLayout {
		case 0:
			return vfsswift.NewThumbsFs(config.GetSwiftConnection(), i.Domain)
		case 1:
			return vfsswift.NewThumbsFsV2(config.GetSwiftConnection(), i)
		case 2:
			return vfsswift.NewThumbsFsV3(config.GetSwiftConnection(), i)
		default:
			panic(instance.ErrInvalidSwiftLayout)
		}
	default:
		panic(fmt.Sprintf("instance: unknown storage provider %s", fsURL.Scheme))
	}
}
