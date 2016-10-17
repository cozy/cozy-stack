// Package vfs is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package vfs

import (
	"path"
	"strings"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/spf13/afero"
)

// ForbiddenFilenameChars is the list of forbidden characters in a filename.
const ForbiddenFilenameChars = "/\x00"

// DocType is the type of document, eg. file or folder
type DocType string

const (
	// FileDocType is document type
	FileDocType DocType = "io.cozy.files"
	// FolderDocType is document type
	FolderDocType = "io.cozy.folders"
)

// ParseDocType is used to transform a string to a DocType.
func ParseDocType(docType string) (result DocType, err error) {
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
func createNewFilePath(name, folderID string, storage afero.Fs, dbPrefix string) (pth string, parentDoc *DirDoc, err error) {
	if err = checkFileName(name); err != nil {
		return
	}

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

	pth = path.Join(parentPath, name)
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
