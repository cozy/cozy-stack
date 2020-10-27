package move

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

type importer struct {
	inst             *instance.Instance
	options          ImportOptions
	doc              *ExportDoc
	appsNotInstalled []string
	tmpFile          string
	doctype          string
	docs             []interface{}
}

func (im *importer) importPart(cursor string) error {
	defer func() {
		if im.tmpFile != "" {
			if err := os.Remove(im.tmpFile); err != nil {
				im.inst.Logger().WithField("nspace", "move").
					Warnf("Cannot remove temp file %s: %s", im.tmpFile, err)
			}
		}
	}()
	if err := im.downloadFile(cursor); err != nil {
		return err
	}
	zr, err := zip.OpenReader(im.tmpFile)
	if err != nil {
		return err
	}
	err = im.importZip(zr.Reader)
	if errc := zr.Close(); err == nil {
		err = errc
	}
	return err
}

func (im *importer) downloadFile(cursor string) error {
	u, err := url.Parse(im.options.ManifestURL)
	if err != nil {
		return err
	}
	u.Path = strings.Replace(u.Path, "/move/exports/", "/move/exports/data/", 1)
	if cursor != "" {
		u.RawQuery = url.Values{"cursor": {cursor}}.Encode()
	}
	res, err := http.Get(u.String())
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ErrExportNotFound
	}
	f, err := ioutil.TempFile("", "export-*")
	if err != nil {
		return err
	}
	im.tmpFile = f.Name()
	_, err = io.Copy(f, res.Body)
	if errc := f.Close(); err == nil {
		err = errc
	}
	return err
}

func (im *importer) importZip(zr zip.Reader) error {
	for _, file := range zr.File {
		if !strings.HasPrefix(file.FileHeader.Name, ExportDataDir) {
			continue
		}
		name := strings.TrimPrefix(file.FileHeader.Name, ExportDataDir)
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("Invalid filename: %s", name)
		}
		doctype := parts[0]
		switch doctype {
		case consts.Exports:
			// Importing exports would just be a mess, so skip them
			continue
		case consts.Sharings, consts.Shared, consts.Permissions:
			// Sharings cannot be imported, they need to be migrated
			continue
		case consts.BitwardenCiphers, consts.BitwardenFolders, consts.BitwardenProfiles:
			// Bitwarden documents are encypted E2E, so they cannot be imported
			// as raw documents
			continue
		case consts.Apps, consts.Konnectors:
			// TODO not yet implemented, they need to be installed
			continue
		case consts.Sessions, consts.Settings:
			// TODO not yet implemented
			continue
		case consts.Triggers:
			// TODO not yet implemented, they need to be filtered to avoid duplicated,
			// and also injected in redis/memory
			continue
		case consts.Files, consts.FilesVersions:
			// TODO not yet implemented, as they have an associated content
			continue
		}
		if doctype != im.doctype || len(im.docs) >= 1000 {
			if err := im.flush(); err != nil {
				return err
			}
		}
		doc, err := im.readDoc(file)
		if err != nil {
			return err
		}
		im.doctype = doctype
		im.docs = append(im.docs, doc)
	}

	return im.flush()
}

func (im *importer) flush() error {
	if len(im.docs) == 0 {
		return nil
	}

	olds := make([]interface{}, len(im.docs))
	if err := couchdb.BulkUpdateDocs(im.inst, im.doctype, im.docs, olds); err != nil {
		return err
	}

	im.doctype = ""
	im.docs = nil
	return nil
}

func (im *importer) readDoc(zf *zip.File) (json.RawMessage, error) {
	r, err := zf.Open()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if errc := r.Close(); errc != nil {
		return nil, errc
	}
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
