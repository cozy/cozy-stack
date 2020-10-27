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

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/contact"
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
		id := parts[1]

		// Special cases
		switch doctype {
		case consts.Exports:
			// Importing exports would just be a mess, so skip them
			continue
		case consts.Sessions:
			// We don't want to import the sessions from another instance
			continue
		case consts.Settings:
			// Keep the email, public name and stuff related to the cloudery
			// from the destination Cozy
			if id == consts.InstanceSettingsID {
				continue
			}
		case consts.Sharings, consts.Shared, consts.Permissions:
			// Sharings cannot be imported, they need to be migrated
			continue
		case consts.BitwardenCiphers, consts.BitwardenFolders, consts.BitwardenProfiles:
			// Bitwarden documents are encypted E2E, so they cannot be imported
			// as raw documents
			continue
		case consts.Apps, consts.Konnectors:
			im.installApp(id)
			continue
		case consts.Triggers:
			// TODO not yet implemented, they need to be filtered to avoid duplicated,
			// and also injected in redis/memory
			continue
		case consts.Files, consts.FilesVersions:
			// TODO not yet implemented, as they have an associated content
			continue
		}

		// Normal documents
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

	if err := im.flush(); err != nil {
		return err
	}

	// Reinject the email address from the destination Cozy in the myself
	// contact document
	if myself, err := contact.GetMyself(im.inst); err == nil {
		if email, err := im.inst.SettingsEMail(); err == nil && email != "" {
			addr, _ := myself.ToMailAddress()
			if addr == nil || addr.Email != email {
				myself.JSONDoc.M["email"] = []map[string]interface{}{
					{
						"address": email,
						"primary": true,
					},
				}
				_ = couchdb.UpdateDoc(im.inst, myself)
			}
		}
	}
	return nil
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

func (im *importer) installApp(id string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return
	}
	doctype := parts[0]
	apptype := consts.WebappType
	if doctype == consts.Konnectors {
		apptype = consts.KonnectorType
	}
	slug := parts[1]
	source := "registry://" + slug + "/stable"

	installer, err := app.NewInstaller(im.inst, app.Copier(apptype, im.inst),
		&app.InstallerOptions{
			Operation:  app.Install,
			Type:       apptype,
			SourceURL:  source,
			Slug:       slug,
			Registries: im.inst.Registries(),
		},
	)
	if err == nil {
		_, err = installer.RunSync()
	}
	if err != nil {
		im.appsNotInstalled = append(im.appsNotInstalled, slug)
	}
}
