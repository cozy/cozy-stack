package vfsswift

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/ncw/swift/v2"
)

// NewAvatarFsV3 creates a new avatar filesystem base on swift.
//
// This version stores the avatar in the same container as the main data
// container.
func NewAvatarFsV3(c *swift.Connection, db prefixer.Prefixer) vfs.Avatarer {
	return &avatarV3{
		c:         c,
		container: swiftV3ContainerPrefix + db.DBPrefix(),
		ctx:       context.Background(),
	}
}

type avatarV3 struct {
	c         *swift.Connection
	container string
	ctx       context.Context
}

func (a *avatarV3) CreateAvatar(contentType string) (io.WriteCloser, error) {
	return a.c.ObjectCreate(a.ctx, a.container, "avatar", true, "", contentType, nil)
}

func (a *avatarV3) ServeAvatarContent(w http.ResponseWriter, req *http.Request) error {
	f, o, err := a.c.ObjectOpen(a.ctx, a.container, "avatar", false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, o["Etag"]))
	w.Header().Set("Content-Type", o["Content-Type"])
	http.ServeContent(w, req, "avatar", unixEpochZero, &backgroundSeeker{f})
	return nil
}
