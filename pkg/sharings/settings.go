package sharings

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

// SharingSettings is the list of destination directories set by the
// different applications.
type SharingSettings struct {
	// AppDestination is following the format: app slug -> doctype -> dirID
	AppDestination      map[string]map[string]string `json:"app_destination"`
	SharingsSettingsID  string                       `json:"_id,omitempty"`
	SharingsSettingsRev string                       `json:"_rev,omitempty"`
	SharedWithMeDirID   string                       `json:"shared_w_me_dir_id,omitempty"`
}

// ID returns the SharingSettings qualified identifier.
func (s SharingSettings) ID() string { return s.SharingsSettingsID }

// Rev returns the SharingSettings revision.
func (s SharingSettings) Rev() string { return s.SharingsSettingsRev }

// DocType returns the SharingSettings doctype.
func (s SharingSettings) DocType() string { return consts.Settings }

// Clone returns a new SharingSettings with the same values.
func (s *SharingSettings) Clone() couchdb.Doc {
	cloned := *s
	cloned.AppDestination = make(map[string]map[string]string)
	for app, dest := range s.AppDestination {
		appDestMap := make(map[string]string)
		for doctype, dirID := range dest {
			appDestMap[doctype] = dirID
		}

		cloned.AppDestination[app] = appDestMap
	}
	return &cloned
}

// SetID changes the SharingSettings qualified identifier.
func (s *SharingSettings) SetID(id string) { s.SharingsSettingsID = id }

// SetRev changes the SharingSettings revision.
func (s *SharingSettings) SetRev(rev string) { s.SharingsSettingsRev = rev }

func createSharingSettingsDocument(ins *instance.Instance) (*SharingSettings, error) {
	s := SharingSettings{}
	err := couchdb.GetDoc(ins, consts.Settings, consts.SharingSettingsID, &s)
	if err != nil {
		s = SharingSettings{
			SharingsSettingsID: consts.SharingSettingsID,
			AppDestination:     make(map[string]map[string]string),
		}
		err = couchdb.CreateNamedDocWithDB(ins, &s)
		if err != nil {
			return nil, err
		}
	}

	return &s, nil
}

// UpdateApplicationDestinationDirID updates the destination settings for the
// provided app.
//
// slug:    the application slug.
// doctype: the doctype to consider.
// dirID:   where to put futur shared documents of the given doctypes.
func UpdateApplicationDestinationDirID(ins *instance.Instance, slug, doctype, dirID string) error {
	s := SharingSettings{}
	err := couchdb.GetDoc(ins, consts.Settings, consts.SharingSettingsID, &s)
	if err != nil {
		sd, errc := createSharingSettingsDocument(ins)
		if errc != nil {
			return errc
		}

		s = *sd
	}

	if _, ok := s.AppDestination[slug]; ok {
		delete(s.AppDestination[slug], doctype)
		s.AppDestination[slug][doctype] = dirID
	} else {
		s.AppDestination[slug] = map[string]string{doctype: dirID}
	}

	err = couchdb.UpdateDoc(ins, &s)
	return err
}

// RetrieveApplicationDestinationDirID retrieves the destination directory for
// the given application and doctype. The default value is the id of the
// "/Shared with Me" directory.
func RetrieveApplicationDestinationDirID(ins *instance.Instance, slug, doctype string) (string, error) {
	s := &SharingSettings{}
	err := couchdb.GetDoc(ins, consts.Settings, consts.SharingSettingsID, s)
	if err != nil {
		s, err = createSharingSettingsDocument(ins)
		if err != nil {
			return "", err
		}

		return retrieveSharedWithMeDirID(ins, s)
	}

	if slug != "" && doctype != "" {
		if dest, ok := s.AppDestination[slug]; ok {
			if dirID, ok := dest[doctype]; ok && dirID != "" {
				if _, err := ins.VFS().DirByID(dirID); err == nil {
					return dirID, nil
				}
			}
		}
	}

	return retrieveSharedWithMeDirID(ins, s)
}

func retrieveSharedWithMeDirID(ins *instance.Instance, s *SharingSettings) (string, error) {
	if id := s.SharedWithMeDirID; id != "" {
		dirDoc, err := ins.VFS().DirByID(id)
		if err != nil {
			return createSharedWithMeDir(ins, s)
		}

		if dirDoc.DirID == consts.TrashDirID {
			return createSharedWithMeDir(ins, s)
		}

		return id, nil
	}

	return createSharedWithMeDir(ins, s)
}

func createSharedWithMeDir(ins *instance.Instance, s *SharingSettings) (string, error) {
	name := ins.Translate("Sharings Shared with Me directory")
	if doc, err := ins.VFS().DirByPath("/" + name); err == nil {
		s.SharedWithMeDirID = doc.ID()
		return s.SharedWithMeDirID, nil
	}

	dirDoc, err := vfs.NewDirDoc(ins.VFS(), name, "", nil)
	if err != nil {
		return "", err
	}

	t := time.Now()
	dirDoc.CreatedAt = t
	dirDoc.UpdatedAt = t

	err = ins.VFS().CreateDir(dirDoc)
	if err != nil {
		return "", nil
	}

	s.SharedWithMeDirID = dirDoc.ID()
	err = couchdb.UpdateDoc(ins, s)
	if err != nil {
		return "", nil
	}

	return s.SharedWithMeDirID, nil
}

var (
	_ couchdb.Doc = &SharingSettings{}
)
