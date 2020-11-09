package move

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	multierror "github.com/hashicorp/go-multierror"
)

type importer struct {
	inst             *instance.Instance
	fs               vfs.VFS
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
	var errm error

	for i, file := range zr.File {
		if !strings.HasPrefix(file.FileHeader.Name, ExportDataDir+"/") {
			continue
		}
		name := strings.TrimPrefix(file.FileHeader.Name, ExportDataDir+"/")
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 {
			continue // "instance.json" for example
		}
		doctype := parts[0]
		id := strings.TrimSuffix(parts[1], ".json")

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
			// from the destination Cozy. Same for the bitwarden settings
			// derived from the passphrase.
			if id == consts.InstanceSettingsID || id == consts.BitwardenSettingsID {
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
			if err := im.importTrigger(file); err != nil {
				errm = multierror.Append(errm, err)
			}
			continue
		case consts.Files:
			var content *zip.File
			if i < len(zr.File)-1 {
				content = zr.File[i+1]
			}
			if err := im.importFile(file, content); err != nil {
				errm = multierror.Append(errm, err)
			}
			continue
		case consts.FilesVersions:
			if i >= len(zr.File)-1 {
				continue
			}
			if err := im.importFileVersion(file, zr.File[i+1]); err != nil {
				errm = multierror.Append(errm, err)
			}
			continue
		}

		// Normal documents
		if doctype != im.doctype || len(im.docs) >= 200 {
			if err := im.flush(); err != nil {
				errm = multierror.Append(errm, err)
				im.docs = nil
				im.doctype = ""
			}
		}
		doc, err := im.readDoc(file)
		if err != nil {
			errm = multierror.Append(errm, err)
			continue
		}
		delete(doc, "_rev")
		im.doctype = doctype
		im.docs = append(im.docs, doc)
	}

	if err := im.flush(); err != nil {
		errm = multierror.Append(errm, err)
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
	return errm
}

func (im *importer) flush() error {
	if len(im.docs) == 0 {
		return nil
	}

	olds := make([]interface{}, len(im.docs))
	err := couchdb.BulkUpdateDocs(im.inst, im.doctype, im.docs, olds)
	if couchdb.IsNoDatabaseError(err) {
		if errc := couchdb.CreateDB(im.inst, im.doctype); errc != nil {
			return errc
		}
		err = couchdb.BulkUpdateDocs(im.inst, im.doctype, im.docs, olds)
	}
	if err != nil {
		time.Sleep(1 * time.Second)
		err = couchdb.BulkUpdateDocs(im.inst, im.doctype, im.docs, olds)
	}
	if err != nil {
		return err
	}

	im.doctype = ""
	im.docs = nil
	return nil
}

func (im *importer) readDoc(zf *zip.File) (map[string]interface{}, error) {
	r, err := zf.Open()
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	err = json.NewDecoder(r).Decode(&doc)
	if errc := r.Close(); errc != nil {
		return nil, errc
	}
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (im *importer) readTrigger(zf *zip.File) (*job.TriggerInfos, error) {
	r, err := zf.Open()
	if err != nil {
		return nil, err
	}
	doc := &job.TriggerInfos{}
	err = json.NewDecoder(r).Decode(doc)
	if errc := r.Close(); errc != nil {
		return nil, errc
	}
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (im *importer) importTrigger(zf *zip.File) error {
	doc, err := im.readTrigger(zf)
	if err != nil {
		return err
	}
	if doc.WorkerType != "konnector" {
		return nil
	}
	doc.SetRev("")
	t, err := job.NewTrigger(im.inst, *doc, nil)
	if err != nil {
		return err
	}
	return job.System().AddTrigger(t)
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

func (im *importer) importFile(zdoc, zcontent *zip.File) error {
	doc, err := im.readFileDoc(zdoc)
	if err != nil {
		return err
	}
	dirDoc, fileDoc := doc.Refine()
	if dirDoc != nil {
		dirDoc.SetRev("")
		if dirDoc.DocID == consts.RootDirID || dirDoc.DocID == consts.TrashDirID {
			return nil
		}
		return im.fs.CreateDir(dirDoc)
	}

	if zcontent == nil {
		return errors.New("No content for file")
	}
	fileDoc.SetRev("")
	f, err := im.fs.CreateFile(fileDoc, nil, vfs.AllowCreationInTrash)
	if err != nil {
		return err
	}

	content, err := zcontent.Open()
	if err != nil {
		return err
	}
	_, err = io.Copy(f, content)
	if errc := f.Close(); err == nil {
		err = errc
	}
	return err
}

func (im *importer) readFileDoc(zf *zip.File) (*vfs.DirOrFileDoc, error) {
	r, err := zf.Open()
	if err != nil {
		return nil, err
	}
	var doc vfs.DirOrFileDoc
	err = json.NewDecoder(r).Decode(&doc)
	if errc := r.Close(); errc != nil {
		return nil, errc
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

func (im *importer) importFileVersion(zdoc, zcontent *zip.File) error {
	doc, err := im.readVersion(zdoc)
	if err != nil {
		return err
	}
	content, err := zcontent.Open()
	if err != nil {
		return err
	}
	doc.SetRev("")
	return im.fs.ImportFileVersion(doc, content)
}

func (im *importer) readVersion(zf *zip.File) (*vfs.Version, error) {
	r, err := zf.Open()
	if err != nil {
		return nil, err
	}
	var doc vfs.Version
	err = json.NewDecoder(r).Decode(&doc)
	if errc := r.Close(); errc != nil {
		return nil, errc
	}
	if err != nil {
		return nil, err
	}
	return &doc, nil
}
