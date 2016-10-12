package files

import (
	"bytes"
	"crypto/md5" // #nosec
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/spf13/afero"
)

// DefaultContentType is used for files uploaded with no content-type
const DefaultContentType = "application/octet-stream"

type fileAttributes struct {
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Size       int64     `json:"size,string"`
	Tags       []string  `json:"tags"`
	MD5Sum     []byte    `json:"md5sum"`
	Executable bool      `json:"executable"`
	Class      string    `json:"class"`
	Mime       string    `json:"mime"`
}

// FileDoc is a struct containing all the informations about a file.
// It implements the couchdb.Doc and jsonapi.JSONApier interfaces.
type FileDoc struct {
	// Qualified file identifier
	QID string `json:"_id"`
	// File revision
	FRev string `json:"_rev,omitempty"`
	// File attributes
	Attrs *fileAttributes `json:"attributes"`
	// Parent folder identifier
	FolderID string `json:"folderID"`
	// File path on VFS
	Path string `json:"path"`
}

// ID returns the file qualified identifier (part of couchdb.Doc
// interface)
func (f *FileDoc) ID() string {
	return f.QID
}

// Rev returns the file revision (part of couchdb.Doc interface)
func (f *FileDoc) Rev() string {
	return f.FRev
}

// DocType returns the file document type (part of couchdb.Doc
// interface)
func (f *FileDoc) DocType() string {
	return string(FileDocType)
}

// SetID is used to change the file qualified identifier (part of
// couchdb.Doc interface)
func (f *FileDoc) SetID(id string) {
	f.QID = id
}

// SetRev is used to change the file revision (part of couchdb.Doc
// interface)
func (f *FileDoc) SetRev(rev string) {
	f.FRev = rev
}

// ToJSONApi implements temporary interface JSONApier to serialize
// the file document
func (f *FileDoc) ToJSONApi() ([]byte, error) {
	qid := f.QID
	data := map[string]interface{}{
		"id":         qid[strings.Index(qid, "/")+1:],
		"type":       f.DocType(),
		"rev":        f.Rev(),
		"attributes": f.Attrs,
	}
	m := map[string]interface{}{
		"data": data,
	}
	return json.Marshal(m)
}

// GetFileDoc is used to fetch file document information form our
// database.
func GetFileDoc(fileID, dbPrefix string) (doc *FileDoc, err error) {
	doc = &FileDoc{}
	err = couchdb.GetDoc(dbPrefix, string(FileDocType), fileID, doc)
	return
}

// StatFile is used to have information about the a file from its
// path.
func StatFile(pth string, fs afero.Fs) (os.FileInfo, error) {
	return fs.Stat(pth)
}

// ReadFile is used to read a file given its path from the filesystem
// into the given writer.
func ReadFile(pth string, fs afero.Fs, w io.Writer) (err error) {
	f, err := fs.Open(pth)
	if err != nil {
		return
	}

	defer f.Close()
	_, err = io.Copy(w, f)

	return
}

// CreateFileAndUpload is the method for uploading a file onto the filesystem.
func CreateFileAndUpload(m *DocMetadata, fs afero.Fs, contentType string, contentLength int64, dbPrefix string, body io.ReadCloser) (doc *FileDoc, err error) {
	if m.Type != FileDocType {
		err = errDocTypeInvalid
		return
	}

	pth, _, err := createNewFilePath(m, fs, dbPrefix)
	if err != nil {
		return
	}

	mime, class := extractMimeAndClass(contentType)
	createDate := time.Now()
	attrs := &fileAttributes{
		Name:       m.Name,
		CreatedAt:  createDate,
		UpdatedAt:  createDate,
		Size:       contentLength,
		Tags:       m.Tags,
		MD5Sum:     m.GivenMD5,
		Executable: m.Executable,
		Class:      class,
		Mime:       mime,
	}

	doc = &FileDoc{
		Attrs:    attrs,
		FolderID: m.FolderID,
		Path:     pth,
	}

	// Error handling to make sure the steps of uploading the file and
	// creating the corresponding are both rollbacked in case of an
	// error. This should preserve our VFS coherency a little.
	defer func() {
		if err != nil {
			fs.Remove(pth)
		}
	}()

	var written int64
	if written, err = copyOnFsAndCheckIntegrity(m, fs, pth, body); err != nil {
		return
	}

	if contentLength >= 0 && written != contentLength {
		err = errContentLengthMismatch
		return
	}

	if contentLength < 0 {
		attrs.Size = written
	}

	if err = couchdb.CreateDoc(dbPrefix, doc.DocType(), doc); err != nil {
		return
	}

	return
}

func copyOnFsAndCheckIntegrity(m *DocMetadata, fs afero.Fs, pth string, r io.ReadCloser) (written int64, err error) {
	f, err := fs.Create(pth)
	if err != nil {
		return
	}

	defer f.Close()
	defer r.Close()

	md5H := md5.New() // #nosec
	written, err = io.Copy(f, io.TeeReader(r, md5H))
	if err != nil {
		return
	}

	calcMD5 := md5H.Sum(nil)
	if !bytes.Equal(m.GivenMD5, calcMD5) {
		err = errInvalidHash
		return
	}

	return
}

func extractMimeAndClass(contentType string) (mime, class string) {
	if contentType == "" {
		contentType = DefaultContentType
	}

	charsetIndex := strings.Index(contentType, ";")
	if charsetIndex >= 0 {
		mime = contentType[:charsetIndex]
	} else {
		mime = contentType
	}

	// @TODO improve for specific mime types
	slashIndex := strings.Index(contentType, "/")
	if slashIndex >= 0 {
		class = contentType[:slashIndex]
	} else {
		class = contentType
	}

	return
}
