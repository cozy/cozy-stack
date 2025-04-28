package vfsswift

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

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
	headers := swift.Metadata{
		"created-at": time.Now().UTC().Format(time.RFC3339),
	}.ObjectHeaders()
	return a.c.ObjectCreate(a.ctx, a.container, "avatar", true, "", contentType, headers)
}

func (a *avatarV3) DeleteAvatar() error {
	err := a.c.ObjectDelete(a.ctx, a.container, "avatar")
	if err == swift.ObjectNotFound {
		return nil
	}
	return err
}

func (a *avatarV3) ServeAvatarContent(w http.ResponseWriter, req *http.Request) error {
	f, o, err := a.c.ObjectOpen(a.ctx, a.container, "avatar", false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	t := unixEpochZero
	if createdAt := o["created-at"]; createdAt != "" {
		if createdAtTime, err := time.Parse(time.RFC3339, createdAt); err == nil {
			t = createdAtTime
		}
	}

	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, o["Etag"]))
	w.Header().Set("Content-Type", o["Content-Type"])
	http.ServeContent(w, req, "avatar", t, &backgroundSeeker{f})
	return nil
}
