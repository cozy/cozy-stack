package vfs

import (
	"sort"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Version is used for storing the metadata about previous versions of file
// contents. The content its-self is stored on the local file system or in
// Swift.
type Version struct {
	DocID        string            `json:"_id,omitempty"`
	DocRev       string            `json:"_rev,omitempty"`
	UpdatedAt    time.Time         `json:"updated_at"`
	ByteSize     int64             `json:"size,string"`
	MD5Sum       []byte            `json:"md5sum"`
	Tags         []string          `json:"tags"`
	Metadata     Metadata          `json:"metadata,omitempty"`
	CozyMetadata FilesCozyMetadata `json:"cozyMetadata,omitempty"`
	Rels         struct {
		File struct {
			Data struct {
				ID   string `json:"_id"`
				Type string `json:"_type"`
			} `json:"data"`
		} `json:"file"`
	} `json:"relationships"`
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
	cloned.Metadata = make(Metadata, len(v.Metadata))
	for k, val := range v.Metadata {
		cloned.Metadata[k] = val
	}
	meta := v.CozyMetadata.Clone()
	cloned.CozyMetadata = *meta
	return &cloned
}

// SetID changes the version qualified identifier
func (v *Version) SetID(id string) { v.DocID = id }

// SetRev changes the version revision
func (v *Version) SetRev(rev string) { v.DocRev = rev }

// Included is part of jsonapi.Object interface
func (v *Version) Included() []jsonapi.Object { return nil }

// Relationships is part of jsonapi.Object interface
func (v *Version) Relationships() jsonapi.RelationshipMap { return nil }

// Links is part of jsonapi.Object interface
func (v *Version) Links() *jsonapi.LinksList { return nil }

// NewVersion returns a version from a given FileDoc. It is often used just
// before modifying the content of this file.
// Note that the _id is precomputed as it can be useful to use it for a storage
// location before the version is saved in CouchDB.
func NewVersion(file *FileDoc) *Version {
	var instanceURL string
	if file.CozyMetadata != nil {
		instanceURL = file.CozyMetadata.UploadedOn
	}
	fcm := NewCozyMetadata(instanceURL)
	id := file.InternalID
	if id == "" {
		id = file.Rev()
	}
	v := &Version{
		DocID:        file.ID() + "/" + id,
		UpdatedAt:    file.UpdatedAt,
		ByteSize:     file.ByteSize,
		MD5Sum:       file.MD5Sum,
		Tags:         file.Tags,
		Metadata:     file.Metadata,
		CozyMetadata: *fcm,
	}
	v.Rels.File.Data.ID = file.ID()
	v.Rels.File.Data.Type = consts.Files
	v.CozyMetadata.UploadedOn = instanceURL
	at := file.UpdatedAt
	if file.CozyMetadata != nil && file.CozyMetadata.UploadedAt != nil {
		at = *file.CozyMetadata.UploadedAt
	}
	v.CozyMetadata.UploadedAt = &at
	if file.CozyMetadata != nil && file.CozyMetadata.UploadedBy != nil {
		by := *file.CozyMetadata.UploadedBy
		v.CozyMetadata.UploadedBy = &by
	}
	return v
}

// SetMetaFromVersion takes the metadata from the version and copies them to
// the file document.
func SetMetaFromVersion(file *FileDoc, version *Version) {
	file.UpdatedAt = version.UpdatedAt
	file.ByteSize = version.ByteSize
	file.MD5Sum = version.MD5Sum
	file.Tags = version.Tags
	file.Metadata = version.Metadata
	if file.CozyMetadata == nil {
		file.CozyMetadata = NewCozyMetadata("")
		file.CozyMetadata.CreatedAt = file.CreatedAt
	}
	file.CozyMetadata.UploadedOn = version.CozyMetadata.UploadedOn
	at := *version.CozyMetadata.UploadedAt
	file.CozyMetadata.UploadedAt = &at
	if version.CozyMetadata.UploadedBy != nil {
		by := *version.CozyMetadata.UploadedBy
		file.CozyMetadata.UploadedBy = &by
	}
}

// FindVersion returns the version for the given id
func FindVersion(db prefixer.Prefixer, id string) (*Version, error) {
	doc := &Version{}
	if err := couchdb.GetDoc(db, consts.FilesVersions, id, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// VersionsFor returns the list of the versions for a given file identifier.
func VersionsFor(db prefixer.Prefixer, fileID string) ([]*Version, error) {
	var versions []*Version
	req := &couchdb.AllDocsRequest{
		StartKey: fileID + "/",
		EndKey:   fileID + "0", // 0 is the next character after / in ascii
	}
	if err := couchdb.GetAllDocs(db, consts.FilesVersions, req, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// FindVersionsToClean returns a bool to say if the candidate version must be
// cleaned, a list of old versions to clean, and an error. The rules to know
// the versions to clean or keep are:
// - the tagged versions are kept
// - two versions must not be too close in time
// - there is a maximal number of versions.
func FindVersionsToClean(db prefixer.Prefixer, fileID string, candidate *Version) (bool, []*Version, error) {
	olds, err := VersionsFor(db, fileID)
	if err != nil {
		return false, nil, err
	}
	cfg := config.GetConfig()
	maxNumber := cfg.Fs.Versioning.MaxNumberToKeep
	minDelay := cfg.Fs.Versioning.MinDelayBetweenTwoVersions
	cleanCandidate, toClean := detectVersionsToClean(candidate, olds, maxNumber, minDelay)
	return cleanCandidate, toClean, nil
}

func detectVersionsToClean(candidate *Version, olds []*Version, maxNumber int, minDelay time.Duration) (bool, []*Version) {
	if len(olds) == 0 {
		return false, nil
	}

	sort.Slice(olds, func(i, j int) bool {
		return olds[i].CozyMetadata.CreatedAt.Before(olds[j].CozyMetadata.CreatedAt)
	})

	// We will keep the candidate version if it has no tags and is not too
	// close to the previous version.
	cleanCandidate := false
	if candidate != nil && len(candidate.Tags) == 0 {
		candidateTime := candidate.CozyMetadata.CreatedAt
		previousTime := olds[len(olds)-1].CozyMetadata.CreatedAt
		if previousTime.Add(minDelay).After(candidateTime) {
			cleanCandidate = true
		}
	}

	// Find the number of old versions to clean
	nb := len(olds) + 1 - maxNumber // +1 is for the current version
	if candidate != nil && !cleanCandidate {
		nb++ // +1 for the candidate version (if it is kept)
	}
	if nb <= 0 {
		return cleanCandidate, nil
	}

	var toClean []*Version
	for _, v := range olds {
		if len(v.Tags) > 0 {
			continue
		}
		toClean = append(toClean, v)
		nb--
		if nb <= 0 {
			break
		}
	}
	return cleanCandidate, toClean
}

var _ jsonapi.Object = &Version{}
