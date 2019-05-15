package vfs

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	uuid "github.com/satori/go.uuid"
)

// Version is used for storing the metadata about previous versions of file
// contents. The content its-self is stored on the local file system or in
// Swift.
type Version struct {
	DocID     string    `json:"_id,omitempty"`
	DocRev    string    `json:"_rev,omitempty"`
	FileID    string    `json:"file_id"`
	UpdatedAt time.Time `json:"updated_at"`
	ByteSize  int64     `json:"size,string"`
	MD5Sum    []byte    `json:"md5sum"`
	Tags      []string  `json:"tags"`
}

// ID returns the version identifier
func (v *Version) ID() string { return v.DocID }

// Rev returns the version revision
func (v *Version) Rev() string { return v.DocRev }

// DocType returns the version document type
func (v *Version) DocType() string { return consts.FilesVersions }

// Clone implements couchdb.Doc
func (v *Version) Clone() couchdb.Doc {
	cloned := *v
	cloned.MD5Sum = make([]byte, len(v.MD5Sum))
	copy(cloned.MD5Sum, v.MD5Sum)
	cloned.Tags = make([]string, len(v.Tags))
	copy(cloned.Tags, v.Tags)
	return &cloned
}

// SetID changes the version qualified identifier
func (v *Version) SetID(id string) { v.DocID = id }

// SetRev changes the version revision
func (v *Version) SetRev(rev string) { v.DocRev = rev }

// NewVersion returns a version from a given FileDoc. It is often used just
// before modifying the content of this file.
// Note that the _id is precomputed as it can be useful to use it for a storage
// location before the version is saved in CouchDB.
func NewVersion(file *FileDoc) *Version {
	id, err := uuid.NewV4()
	if err != nil {
		return nil
	}
	return &Version{
		DocID:     id.String(),
		FileID:    file.ID(),
		UpdatedAt: file.UpdatedAt,
		ByteSize:  file.ByteSize,
		MD5Sum:    file.MD5Sum,
		Tags:      file.Tags,
	}
}

var _ couchdb.Doc = &Version{}
