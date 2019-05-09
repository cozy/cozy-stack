package move

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/utils"
	vcardParser "github.com/emersion/go-vcard"
)

func createContact(fs vfs.VFS, hdr *tar.Header, tr *tar.Reader, db prefixer.Prefixer) error {
	decoder := vcardParser.NewDecoder(tr)
	vcard, err := decoder.Decode()
	if err != nil {
		return err
	}

	c := contact.New()
	fullname := "John Doe"
	contactname := map[string]interface{}{
		"givenName":  "John",
		"familyName": "Doe",
	}
	name := vcard.Name()
	if name != nil {
		contactname = map[string]interface{}{
			"familyName":     name.FamilyName,
			"givenName":      name.GivenName,
			"additionalName": name.AdditionalName,
			"namePrefix":     name.HonorificPrefix,
			"nameSuffix":     name.HonorificSuffix,
		}
		fullname = name.Value
	}
	if names := vcard.FormattedNames(); len(names) > 0 {
		fullname = names[0].Value
	}
	c.M["fullname"] = fullname
	c.M["name"] = contactname

	if field := vcard.Get("BDAY"); field != nil {
		c.M["birthday"] = field.Value
	}

	if field := vcard.Get("NOTE"); field != nil {
		c.M["note"] = field.Value
	}

	var emails []map[string]interface{}
	for _, mail := range vcard.Values("EMAIL") {
		email := map[string]interface{}{"address": mail}
		emails = append(emails, email)
	}
	if len(emails) > 0 {
		c.M["email"] = emails
	}

	var phones []map[string]interface{}
	for _, tel := range vcard.Values("TEL") {
		phone := map[string]interface{}{"number": tel}
		phones = append(phones, phone)
	}
	if len(phones) > 0 {
		c.M["phone"] = phones
	}

	var addresses []map[string]interface{}
	for _, address := range vcard.Addresses() {
		a := map[string]interface{}{
			"street":           address.StreetAddress,
			"pobox":            address.PostOfficeBox,
			"city":             address.Locality,
			"region":           address.Region,
			"postcode":         address.PostalCode,
			"country":          address.Country,
			"formattedAddress": address.Value,
		}
		addresses = append(addresses, a)
	}
	if len(addresses) > 0 {
		c.M["address"] = addresses
	}

	return couchdb.CreateDoc(db, c)
}

func createAlbums(i *instance.Instance, tr *tar.Reader, albums *AlbumReferences) error {
	bs := bufio.NewScanner(tr)

	for bs.Scan() {
		jsondoc := &couchdb.JSONDoc{}
		if err := jsondoc.UnmarshalJSON(bs.Bytes()); err != nil {
			return err
		}
		delete(jsondoc.M, "type")
		id := jsondoc.ID()
		jsondoc.SetID("")
		jsondoc.SetRev("")
		jsondoc.Type = consts.PhotosAlbums

		if err := couchdb.CreateDoc(i, jsondoc); err != nil {
			return err
		}
		(*albums)[id] = couchdb.DocReference{
			ID:   jsondoc.ID(),
			Type: consts.PhotosAlbums,
		}
	}

	return bs.Err()
}

// AlbumReferences is used to associate photos to their albums, though we don't
// force the ID of the albums to the values in the tarball.
type AlbumReferences map[string]couchdb.DocReference

func fillAlbums(i *instance.Instance, tr *tar.Reader, dstDoc *vfs.DirDoc, albums *AlbumReferences) error {
	fs := i.VFS()
	bs := bufio.NewScanner(tr)

	for bs.Scan() {
		ref := Reference{}
		if err := json.Unmarshal(bs.Bytes(), &ref); err != nil {
			return err
		}

		file, err := fs.FileByPath(dstDoc.Fullpath + ref.Filepath)
		if err != nil {
			// XXX Ignore missing photos (we have this for migrating some cozy v2)
			continue
		}

		if docRef, ok := (*albums)[ref.Albumid]; ok {
			file.AddReferencedBy(docRef)
			if err = couchdb.UpdateDoc(i, file); err != nil {
				return err
			}
		}
	}

	return bs.Err()
}

func createFile(fs vfs.VFS, hdr *tar.Header, tr *tar.Reader, dstDoc *vfs.DirDoc, dirs map[string]*vfs.DirDoc) error {
	var err error
	name := strings.TrimPrefix(hdr.Name, "files/")
	filename := path.Base(name)
	mime, class := vfs.ExtractMimeAndClassFromFilename(filename)
	now := time.Now()
	executable := hdr.FileInfo().Mode()&0100 != 0

	dirname := path.Join(dstDoc.Fullpath, path.Dir(name))
	dirDoc, ok := dirs[dirname]
	if !ok {
		// XXX Tarball from cozy v2 exports can have files in a non-existent directory
		if dirDoc, err = vfs.MkdirAll(fs, dirname); err != nil {
			return err
		}
		dirs[dirname] = dirDoc
	}
	fileDoc, err := vfs.NewFileDoc(filename, dirDoc.ID(), hdr.Size, nil, mime, class, now, executable, false, nil)
	if err != nil {
		return err
	}

	file, err := fs.CreateFile(fileDoc, nil)
	if err != nil {
		ext := path.Ext(fileDoc.DocName)
		fileName := fileDoc.DocName[0 : len(fileDoc.DocName)-len(ext)]
		fileDoc.DocName = fmt.Sprintf("%s-conflict-%s%s", fileName, utils.RandomString(10), ext)
		file, err = fs.CreateFile(fileDoc, nil)
		if err != nil {
			return err
		}
	}

	_, err = io.Copy(file, tr)
	cerr := file.Close()
	if err != nil {
		return err
	}
	return cerr
}

// untar untar doc directory
func untar(r io.Reader, dst *vfs.DirDoc, instance *instance.Instance) error {
	fs := instance.VFS()

	// tar+gzip reader
	gr, err := gzip.NewReader(r)
	if err != nil {
		logger.WithDomain(instance.Domain).Errorf("Can't open gzip reader for import: %s", err)
		return err
	}
	defer gr.Close()
	tgz := tar.NewReader(gr)

	albumsRef := make(AlbumReferences)
	dirs := make(map[string]*vfs.DirDoc)

	for {
		hdr, errb := tgz.Next()
		if errb == io.EOF {
			break
		}
		if errb != nil {
			logger.WithDomain(instance.Domain).Errorf("Error on import: %s", errb)
			return errb
		}

		parts := strings.SplitN(path.Clean(hdr.Name), "/", 2)
		var name, doctype string
		if len(parts) > 1 {
			doctype = parts[0]
			name = parts[1]
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if doctype == "files" {
				dirname := path.Join(dst.Fullpath, name)
				var dir *vfs.DirDoc
				if _, ok := dirs[dirname]; ok {
					continue
				}
				parentName := path.Join(dst.Fullpath, path.Dir(name))
				if parent, ok := dirs[parentName]; ok {
					dir, err = vfs.NewDirDocWithParent(path.Base(name), parent, nil)
					if err == nil {
						err = fs.CreateDir(dir)
					}
				} else {
					dir, err = vfs.MkdirAll(fs, dirname)
				}
				if err != nil {
					logger.WithDomain(instance.Domain).Errorf("Can't import directory %s: %s", hdr.Name, err)
				} else {
					dirs[dirname] = dir
				}
			}

		case tar.TypeReg:
			if doctype == "albums" && name == albumsFile {
				err = createAlbums(instance, tgz, &albumsRef)
				if err != nil {
					logger.WithDomain(instance.Domain).Errorf("Can't import album %s: %s", hdr.Name, err)
				}
			} else if doctype == "albums" && name == referencesFile {
				err = fillAlbums(instance, tgz, dst, &albumsRef)
				if err != nil {
					logger.WithDomain(instance.Domain).Errorf("Can't import album %s: %s", hdr.Name, err)
				}
			} else if doctype == "contacts" {
				if err = createContact(fs, hdr, tgz, instance); err != nil {
					logger.WithDomain(instance.Domain).Errorf("Can't import contact %s: %s", hdr.Name, err)
				}
			} else if doctype == "files" {
				if err = createFile(fs, hdr, tgz, dst, dirs); err != nil {
					logger.WithDomain(instance.Domain).Errorf("Can't import file %s: %s", hdr.Name, err)
				}
			}

		default:
			logger.WithDomain(instance.Domain).Errorf("Unknown typeflag for import: %v", hdr.Typeflag)
			return errors.New("Unknown typeflag")
		}
	}

	return err
}

// Import is used to import a tarball with files, photos, contacts to an instance
func Import(i *instance.Instance, filename, destination string, increaseQuota bool) error {
	r, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer r.Close()

	fs := i.VFS()
	exist, err := vfs.DirExists(fs, destination)
	if err != nil {
		logger.WithDomain(i.Domain).Errorf("Error for destination %s: %s", destination, err)
		return err
	}
	var dst *vfs.DirDoc
	if !exist {
		dst, err = vfs.Mkdir(fs, destination, nil)
		if err != nil {
			logger.WithDomain(i.Domain).Errorf("Can't create destination directory %s: %s", destination, err)
			return err
		}
	} else {
		dst, err = fs.DirByPath(destination)
		if err != nil {
			logger.WithDomain(i.Domain).Errorf("Can't find destination directory %s: %s", destination, err)
			return err
		}
	}

	// If increaseQuota flag is activated, the disk quota limit is lifted for
	// the import, and when finished, we put it again a quota (the old one if
	// it is enough or a new one based on the usage if we need to increase it)
	oldQuota := i.BytesDiskQuota
	if increaseQuota && oldQuota != 0 {
		i.BytesDiskQuota = 0
		defer func() {
			i.BytesDiskQuota = oldQuota
			usage, err := fs.DiskUsage()
			if err != nil {
				return
			}
			usage = (usage/1e9 + 1) * 1e9 // Round to the superior Go
			if usage > oldQuota {
				_ = lifecycle.Patch(i, &lifecycle.Options{DiskQuota: usage})
			}
		}()
	}

	return untar(r, dst, i)
}
