package vfs

import (
	"encoding/base64"
	"path"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/spf13/afero"
)

// DocType is the type of document, eg. file or folder
type DocType string

const (
	// FileDocType is document type
	FileDocType DocType = "io.cozy.files"
	// FolderDocType is document type
	FolderDocType = "io.cozy.folders"
)

// ForbiddenFilenameChars is the list of forbidden characters in a filename.
const ForbiddenFilenameChars = "/\x00"

// DocAttributes encapsulates the few metadata linked to a document
// creation request.
type DocAttributes struct {
	docType    DocType
	name       string
	folderID   string
	executable bool
	tags       []string
	givenMD5   []byte
	size       int64
	mime       string
	class      string
}

// DocType returns the document type of the attributes.
func (d *DocAttributes) DocType() DocType {
	return d.docType
}

// NewDocAttributes is the DocAttributes constructor. All inputs are
// validated and if wrong, an error is returned.
func NewDocAttributes(docTypeStr, name, folderID, tagsStr, md5Str, contentType, contentLength string, executable bool) (m *DocAttributes, err error) {
	docType, err := parseDocType(docTypeStr)
	if err != nil {
		return
	}

	if err = checkFileName(name); err != nil {
		return
	}

	// FolderID is not mandatory. If empty, the document is at the root
	// of the FS
	if folderID != "" {
		if err = checkFileName(folderID); err != nil {
			return
		}
	}

	tags := parseTags(tagsStr)

	var givenMD5 []byte
	if md5Str != "" {
		givenMD5, err = parseMD5Hash(md5Str)
		if err != nil {
			return
		}
	}

	size, err := parseContentLength(contentLength)
	if err != nil {
		return
	}

	mime, class := extractMimeAndClass(contentType)

	m = &DocAttributes{
		docType:    docType,
		name:       name,
		folderID:   folderID,
		tags:       tags,
		executable: executable,
		givenMD5:   givenMD5,
		size:       size,
		mime:       mime,
		class:      class,
	}

	return
}

func parseTags(str string) (tags []string) {
	for _, tag := range strings.Split(str, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return
}

func parseDocType(docType string) (result DocType, err error) {
	switch docType {
	case "io.cozy.files":
		result = FileDocType
	case "io.cozy.folders":
		result = FolderDocType
	default:
		err = ErrDocTypeInvalid
	}
	return
}

func parseMD5Hash(md5B64 string) ([]byte, error) {
	// Encoded md5 hash in base64 should at least have 22 caracters in
	// base64: 16*3/4 = 21+1/3
	//
	// The padding may add up to 2 characters (non useful). If we are
	// out of these boundaries we know we don't have a good hash and we
	// can bail immediatly.
	if len(md5B64) < 22 || len(md5B64) > 24 {
		return nil, ErrInvalidHash
	}

	givenMD5, err := base64.StdEncoding.DecodeString(md5B64)
	if err != nil || len(givenMD5) != 16 {
		return nil, ErrInvalidHash
	}

	return givenMD5, nil
}

func parseContentLength(contentLength string) (size int64, err error) {
	if contentLength == "" {
		size = -1
		return
	}

	size, err = strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		err = ErrContentLengthInvalid
	}
	return
}

func checkFileName(str string) error {
	if str == "" || strings.ContainsAny(str, ForbiddenFilenameChars) {
		return ErrIllegalFilename
	}
	return nil
}

// checkParentFolderID is used to generate the filepath of a new file
// or directory. It will check if the given parent folderID is well
// defined is the database and filesystem and it will generate the new
// path of the wanted file, checking if there is not colision with
// existing file.
func createNewFilePath(m *DocAttributes, storage afero.Fs, dbPrefix string) (pth string, parentDoc *DirDoc, err error) {
	folderID := m.folderID

	var parentPath string

	if folderID == "" {
		parentPath = "/"
	} else {
		parentDoc = &DirDoc{}

		// NOTE: we only check the existence of the folder on the db
		err = couchdb.GetDoc(dbPrefix, string(FolderDocType), folderID, parentDoc)
		if couchdb.IsNotFoundError(err) {
			err = ErrParentDoesNotExist
		}
		if err != nil {
			return
		}

		parentPath = parentDoc.Path
	}

	pth = path.Join(parentPath, m.name)
	exists, err := afero.Exists(storage, pth)
	if err != nil {
		return
	}
	if exists {
		err = ErrDocAlreadyExists
		return
	}

	return
}
