package lifecycle

import (
	"os"
	"strings"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/utils"
	multierror "github.com/hashicorp/go-multierror"
)

func update(inst *instance.Instance) error {
	if err := couchdb.UpdateDoc(couchdb.GlobalDB, inst); err != nil {
		inst.Logger().Errorf("Could not update: %s", err.Error())
		return err
	}
	return nil
}

func installApp(inst *instance.Instance, slug string) error {
	source := "registry://" + slug + "/stable"
	installer, err := app.NewInstaller(inst, inst.AppsCopier(consts.WebappType), &app.InstallerOptions{
		Operation:  app.Install,
		Type:       consts.WebappType,
		SourceURL:  source,
		Slug:       slug,
		Registries: inst.Registries(),
	})
	if err != nil {
		return err
	}
	_, err = installer.RunSync()
	return err
}

func defineViewsAndIndex(inst *instance.Instance) error {
	if err := couchdb.DefineIndexes(inst, couchdb.Indexes); err != nil {
		return err
	}
	if err := couchdb.DefineViews(inst, couchdb.Views); err != nil {
		return err
	}
	inst.IndexViewsVersion = couchdb.IndexViewsVersion
	return nil
}

func createDefaultFilesTree(inst *instance.Instance) error {
	var errf error

	createDir := func(dir *vfs.DirDoc, err error) (*vfs.DirDoc, error) {
		if err != nil {
			errf = multierror.Append(errf, err)
			return nil, err
		}
		err = inst.VFS().CreateDir(dir)
		if err != nil && !os.IsExist(err) {
			errf = multierror.Append(errf, err)
			return nil, err
		}
		return dir, nil
	}

	name := inst.Translate("Tree Administrative")
	_, _ = createDir(vfs.NewDirDocWithPath(name, consts.RootDirID, "/", nil))

	// Check if we create the "Photos" folder and its subfolders. By default, we
	// are creating it, but some contexts may not want to create them.
	ctxSettings, err := inst.SettingsContext()
	if err != nil && err != instance.ErrContextNotFound {
		return err
	}

	createPhotosFolder := true
	if photosFolderParam, ok := ctxSettings["init_photos_folder"]; ok {
		createPhotosFolder = photosFolderParam.(bool)
	}

	if createPhotosFolder {
		name = inst.Translate("Tree Photos")
		photos, err := createDir(vfs.NewDirDocWithPath(name, consts.RootDirID, "/", nil))
		if err == nil {
			name = inst.Translate("Tree Uploaded from Cozy Photos")
			_, _ = createDir(vfs.NewDirDoc(inst.VFS(), name, photos.ID(), nil))
			name = inst.Translate("Tree Backed up from my mobile")
			_, _ = createDir(vfs.NewDirDoc(inst.VFS(), name, photos.ID(), nil))
		}
	}

	return errf
}

func checkAliases(inst *instance.Instance, aliases []string) ([]string, error) {
	if aliases == nil {
		return nil, nil
	}
	aliases = utils.UniqueStrings(aliases)
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if alias == inst.Domain {
			return nil, instance.ErrExists
		}
		other, err := instance.GetFromCouch(alias)
		if err != instance.ErrNotFound {
			if err != nil {
				return nil, err
			}
			if other.ID() != inst.ID() {
				return nil, instance.ErrExists
			}
		}
	}
	return aliases, nil
}

const illegalChars = " /,;&?#@|='\"\t\r\n\x00"
const illegalFirstChars = "0123456789."

func validateDomain(domain string) (string, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" || domain == ".." || domain == "." {
		return "", instance.ErrIllegalDomain
	}
	if strings.ContainsAny(domain, illegalChars) {
		return "", instance.ErrIllegalDomain
	}
	if strings.ContainsAny(domain[:1], illegalFirstChars) {
		return "", instance.ErrIllegalDomain
	}
	domain = strings.ToLower(domain)
	if config.GetConfig().Subdomains == config.FlatSubdomains {
		parts := strings.SplitN(domain, ".", 2)
		if strings.Contains(parts[0], "-") {
			return "", instance.ErrIllegalDomain
		}
	}
	return domain, nil
}
