package imexport

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
	vcardParser "github.com/emersion/go-vcard"
)

const (
	metaDir    = "metadata"
	contactExt = ".vcf"
)

// ContactName is a struct describing a name of a contact
type ContactName struct {
	FamilyName     string `json:"familyName,omitempty"`
	GivenName      string `json:"givenName,omitempty"`
	AdditionalName string `json:"additionalName,omitempty"`
	NamePrefix     string `json:"namePrefix,omitempty"`
	NameSuffix     string `json:"nameSuffix,omitempty"`
}

// ContactEmail is a struct describing an email of a contact
type ContactEmail struct {
	Address string `json:"address"`
	Type    string `json:"type,omitempty"`
	Label   string `json:"label,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// ContactAddress is a struct describing an address of a contact
type ContactAddress struct {
	Street           string `json:"street,omitempty"`
	Pobox            string `json:"pobox,omitempty"`
	City             string `json:"city,omitempty"`
	Region           string `json:"region,omitempty"`
	Postcode         string `json:"postcode,omitempty"`
	Country          string `json:"country,omitempty"`
	Type             string `json:"type,omitempty"`
	Primary          bool   `json:"primary,omitempty"`
	Label            string `json:"label,omitempty"`
	FormattedAddress string `json:"formattedAddress,omitempty"`
}

// ContactPhone is a struct describing a phone of a contact
type ContactPhone struct {
	Number  string `json:"number"`
	Type    string `json:"type,omitempty"`
	Label   string `json:"label,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// ContactCozy is a struct describing a cozy instance of a contact
type ContactCozy struct {
	URL     string `json:"url"`
	Label   string `json:"label,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// Contact is a struct containing all the informations about a contact
type Contact struct {
	DocID  string `json:"_id,omitempty"`
	DocRev string `json:"_rev,omitempty"`

	FullName string            `json:"fullname,omitempty"`
	Name     *ContactName      `json:"name,omitempty"`
	Email    []*ContactEmail   `json:"email,omitempty"`
	Address  []*ContactAddress `json:"address,omitempty"`
	Phone    []*ContactPhone   `json:"phone,omitempty"`
	Cozy     []*ContactCozy    `json:"cozy,omitempty"`
}

// ID returns the contact qualified identifier
func (c *Contact) ID() string { return c.DocID }

// Rev returns the contact revision
func (c *Contact) Rev() string { return c.DocRev }

// DocType returns the contact document type
func (c *Contact) DocType() string { return consts.Contacts }

// Clone implements couchdb.Doc
func (c *Contact) Clone() couchdb.Doc {
	cloned := *c
	cloned.FullName = c.FullName
	cloned.Name = c.Name

	cloned.Email = make([]*ContactEmail, len(c.Email))
	copy(cloned.Email, c.Email)

	cloned.Address = make([]*ContactAddress, len(c.Address))
	copy(cloned.Address, c.Address)

	cloned.Phone = make([]*ContactPhone, len(c.Phone))
	copy(cloned.Phone, c.Phone)

	cloned.Cozy = make([]*ContactCozy, len(c.Cozy))
	copy(cloned.Cozy, c.Cozy)

	return &cloned
}

// SetID changes the contact qualified identifier
func (c *Contact) SetID(id string) { c.DocID = id }

// SetRev changes the contact revision
func (c *Contact) SetRev(rev string) { c.DocRev = rev }

func createAlbum(fs vfs.VFS, hdr *tar.Header, tr *tar.Reader, dstDoc *vfs.DirDoc, db couchdb.Database) error {
	m := make(map[string]*couchdb.DocReference)

	bs := bufio.NewScanner(tr)

	for bs.Scan() {
		jsondoc := &couchdb.JSONDoc{}
		err := jsondoc.UnmarshalJSON(bs.Bytes())
		if err != nil {
			return err
		}
		doctype, ok := jsondoc.M["type"].(string)
		if ok {
			jsondoc.Type = doctype
		}
		delete(jsondoc.M, "type")

		id := jsondoc.ID()
		jsondoc.SetID("")
		jsondoc.SetRev("")

		err = couchdb.CreateDoc(db, jsondoc)
		if err != nil {
			return err
		}

		m[id] = &couchdb.DocReference{
			ID:   jsondoc.ID(),
			Type: jsondoc.DocType(),
		}

	}

	_, err := tr.Next()
	if err != nil {
		return err
	}

	bs = bufio.NewScanner(tr)
	for bs.Scan() {
		ref := &References{}
		err := json.Unmarshal(bs.Bytes(), &ref)
		if err != nil {
			return err
		}

		file, err := fs.FileByPath(dstDoc.Fullpath + ref.Filepath)
		if err != nil {
			return err
		}

		if m[ref.Albumid] != nil {
			file.AddReferencedBy(*m[ref.Albumid])
			if err = couchdb.UpdateDoc(db, file); err != nil {
				return err
			}
		}

	}

	return nil

}

func createFile(fs vfs.VFS, hdr *tar.Header, tr *tar.Reader, dstDoc *vfs.DirDoc) error {
	name := path.Base(hdr.Name)
	mime, class := vfs.ExtractMimeAndClassFromFilename(hdr.Name)
	now := time.Now()
	executable := hdr.FileInfo().Mode()&0100 != 0

	dirDoc, err := fs.DirByPath(path.Join(dstDoc.Fullpath, path.Dir(hdr.Name)))
	if err != nil {
		return err
	}

	fileDoc, err := vfs.NewFileDoc(name, dirDoc.ID(), hdr.Size, nil, mime, class, now, executable, false, nil)
	if err != nil {
		return err
	}

	file, err := fs.CreateFile(fileDoc, nil)
	if err != nil {
		if strings.Contains(path.Dir(hdr.Name), "/Photos/") {
			return nil
		}
		extension := path.Ext(fileDoc.DocName)
		fileName := fileDoc.DocName[0 : len(fileDoc.DocName)-len(extension)]
		fileDoc.DocName = fmt.Sprintf("%s-conflict-%d%s", fileName, rand.Int(), extension)
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
	if cerr != nil {
		return cerr
	}

	return nil
}

func createContact(fs vfs.VFS, hdr *tar.Header, tr *tar.Reader, db couchdb.Database) error {

	decoder := vcardParser.NewDecoder(tr)
	vcard, err := decoder.Decode()
	if err != nil {
		return err
	}

	name := vcard.Name()
	contactname := &ContactName{
		FamilyName:     name.FamilyName,
		GivenName:      name.GivenName,
		AdditionalName: name.AdditionalName,
		NamePrefix:     name.HonorificPrefix,
		NameSuffix:     name.HonorificSuffix,
	}

	var contactemail []*ContactEmail
	for i, mail := range vcard.Values("EMAIL") {
		ce := &ContactEmail{
			Address: mail,
		}
		if i == 0 {
			ce.Type = "MAIN"
			ce.Primary = true
		}
		contactemail = append(contactemail, ce)
	}

	var contactphone []*ContactPhone
	for i, phone := range vcard.Values("TEL") {
		cp := &ContactPhone{
			Number: phone,
		}
		if i == 0 {
			cp.Type = "MAIN"
			cp.Primary = true
		}
		contactphone = append(contactphone, cp)
	}

	var contactaddress []*ContactAddress
	for _, address := range vcard.Addresses() {
		ca := &ContactAddress{
			Street:           address.StreetAddress,
			Pobox:            address.PostOfficeBox,
			City:             address.Locality,
			Region:           address.Region,
			Postcode:         address.PostalCode,
			Country:          address.Country,
			FormattedAddress: address.Value,
		}
		contactaddress = append(contactaddress, ca)
	}

	contact := &Contact{
		FullName: name.Value,
		Name:     contactname,
		Address:  contactaddress,
		Email:    contactemail,
		Phone:    contactphone,
	}

	return couchdb.CreateDoc(db, contact)

}

// Untardir untar doc directory
func Untardir(r io.Reader, dst string, instance *instance.Instance) error {
	fs := instance.VFS()
	domain := instance.Domain
	db := couchdb.SimpleDatabasePrefix(domain)

	dstDoc, err := fs.DirByID(dst)
	if err != nil {
		return err
	}

	//gzip reader
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()

	//tar reader
	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		doc := path.Join(dstDoc.Fullpath, hdr.Name)

		switch hdr.Typeflag {

		case tar.TypeDir:
			fmt.Println(hdr.Name)
			if !strings.Contains(hdr.Name, metaDir) {

				if _, err = vfs.MkdirAll(fs, doc, nil); err != nil {
					return err
				}
			}

		case tar.TypeReg:

			if path.Base(hdr.Name) == albumFile {
				err = createAlbum(fs, hdr, tr, dstDoc, db)
				if err != nil {
					return err
				}
			} else if path.Ext(hdr.Name) == contactExt {
				if err := createContact(fs, hdr, tr, db); err != nil {
					return err
				}
			} else {
				if err := createFile(fs, hdr, tr, dstDoc); err != nil {
					return err
				}
			}

		default:
			fmt.Println("Unknown typeflag", hdr.Typeflag)
			return errors.New("Unknown typeflag")

		}

	}

	return nil

}
