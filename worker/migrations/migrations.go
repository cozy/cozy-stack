package migrations

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/note"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/model/vfs/vfsswift"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/ncw/swift"
)

const (
	swiftV1ToV2 = "swift-v1-to-v2"
	toSwiftV3   = "to-swift-v3"

	swiftV1ContainerPrefixCozy = "cozy-"
	swiftV1ContainerPrefixData = "data-"
	swiftV2ContainerPrefixCozy = "cozy-v2-"
	swiftV2ContainerPrefixData = "data-v2-"
	swiftV3ContainerPrefix     = "cozy-v3-"

	accountsToOrganization = "accounts-to-organization"
	notesMimeType          = "notes-mime-type"
)

// maxSimultaneousCalls is the maximal number of simultaneous calls to Swift
// made by a single migration.
const maxSimultaneousCalls = 8

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "migrations",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Reserved:     true,
		WorkerFunc:   worker,
		WorkerCommit: commit,
		Timeout:      6 * time.Hour,
	})
}

type message struct {
	Type string `json:"type"`
}

func worker(ctx *job.WorkerContext) error {
	var msg message
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}

	logger.WithDomain(ctx.Instance.Domain).WithField("nspace", "migration").
		Infof("Start the migration %s", msg.Type)

	switch msg.Type {
	case toSwiftV3:
		return migrateToSwiftV3(ctx.Instance.Domain)
	case swiftV1ToV2:
		return fmt.Errorf("this migration type is no longer supported")
	case accountsToOrganization:
		return migrateAccountsToOrganization(ctx.Instance.Domain)
	case notesMimeType:
		return migrateNotesMimeType(ctx.Instance.Domain)
	default:
		return fmt.Errorf("unknown migration type %q", msg.Type)
	}
}

func commit(ctx *job.WorkerContext, err error) error {
	var msg message
	var migrationType string

	if msgerr := ctx.UnmarshalMessage(&msg); msgerr != nil {
		migrationType = ""
	} else {
		migrationType = msg.Type
	}

	log := logger.WithDomain(ctx.Instance.Domain).WithField("nspace", "migration")
	if err == nil {
		log.Infof("Migration %s success", migrationType)
	} else {
		log.Errorf("Migration %s error: %s", migrationType, err)
	}
	return err
}

func migrateNotesMimeType(domain string) error {
	inst, err := instance.GetFromCouch(domain)
	if err != nil {
		return err
	}
	log := inst.Logger().WithField("nspace", "migration")

	var docs []*vfs.FileDoc
	req := &couchdb.FindRequest{
		UseIndex: "by-mime-updated-at",
		Selector: mango.And(
			mango.Equal("mime", "text/markdown"),
			mango.Exists("updated_at"),
		),
		Limit: 1000,
	}
	_, err = couchdb.FindDocsRaw(inst, consts.Files, req, &docs)
	if err != nil {
		return err
	}
	log.Infof("Found %d markdown files", len(docs))
	for _, doc := range docs {
		if _, ok := doc.Metadata["version"]; !ok {
			log.Infof("Skip file %#v", doc)
			continue
		}
		if err := note.Update(inst, doc.ID()); err != nil {
			log.Warnf("Cannot change mime-type for note %s: %s", doc.ID(), err)
		}
	}

	return nil
}

func migrateToSwiftV3(domain string) error {
	c := config.GetSwiftConnection()
	inst, err := instance.GetFromCouch(domain)
	if err != nil {
		return err
	}
	log := inst.Logger().WithField("nspace", "migration")

	var srcContainer, migratedFrom string
	switch inst.SwiftLayout {
	case 0: // layout v1
		srcContainer = swiftV1ContainerPrefixCozy + inst.DBPrefix()
		migratedFrom = "v1"
	case 1: // layout v2
		srcContainer = swiftV2ContainerPrefixCozy + inst.DBPrefix()
		switch inst.DBPrefix() {
		case inst.Domain:
			migratedFrom = "v2a"
		case inst.Prefix:
			migratedFrom = "v2b"
		default:
			return instance.ErrInvalidSwiftLayout
		}
	case 2: // layout v3
		return nil // Nothing to do!
	default:
		return instance.ErrInvalidSwiftLayout
	}

	log.Infof("Migrating from swift layout %s to swift layout v3", migratedFrom)

	vfs := inst.VFS()
	root, err := vfs.DirByID(consts.RootDirID)
	if err != nil {
		return err
	}

	mutex := lock.LongOperation(inst, "vfs")
	if err = mutex.Lock(); err != nil {
		return err
	}
	defer mutex.Unlock()

	dstContainer := swiftV3ContainerPrefix + inst.DBPrefix()
	if _, _, err = c.Container(dstContainer); err != swift.ContainerNotFound {
		log.Errorf("Destination container %s already exists or something went wrong. Migration canceled.", dstContainer)
		return errors.New("Destination container busy")
	}
	if err = c.ContainerCreate(dstContainer, nil); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if err := vfsswift.DeleteContainer(c, dstContainer); err != nil {
				log.Errorf("Failed to delete v3 container %s: %s", dstContainer, err)
			}
		}
	}()

	if err = copyTheFilesToSwiftV3(inst, c, root, srcContainer, dstContainer); err != nil {
		return err
	}

	meta := &swift.Metadata{"cozy-migrated-from": migratedFrom}
	_ = c.ContainerUpdate(dstContainer, meta.ContainerHeaders())
	if in, err := instance.GetFromCouch(domain); err == nil {
		inst = in
	}
	inst.SwiftLayout = 2
	if err = couchdb.UpdateDoc(couchdb.GlobalDB, inst); err != nil {
		return err
	}

	// Migration done. Now clean-up oldies.

	// WARNING: Don't call `err` any error below in this function or the defer func
	//          will delete the new container even if the migration was successful

	if deleteErr := vfs.Delete(); deleteErr != nil {
		log.Errorf("Failed to delete old %s containers: %s", migratedFrom, deleteErr)
	}
	return nil
}

func copyTheFilesToSwiftV3(inst *instance.Instance, c *swift.Connection, root *vfs.DirDoc, src, dst string) error {
	log := logger.WithDomain(inst.Domain).
		WithField("nspace", "migration")
	sem := semaphore.NewWeighted(maxSimultaneousCalls)
	g, ctx := errgroup.WithContext(context.Background())
	fs := inst.VFS()

	var thumbsContainer string
	switch inst.SwiftLayout {
	case 0: // layout v1
		thumbsContainer = swiftV1ContainerPrefixData + inst.Domain
	case 1: // layout v2
		thumbsContainer = swiftV2ContainerPrefixData + inst.DBPrefix()
	default:
		return instance.ErrInvalidSwiftLayout
	}

	errw := vfs.WalkAlreadyLocked(fs, root, func(_ string, d *vfs.DirDoc, f *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}
		if f == nil {
			return nil
		}
		srcName := getSrcName(inst, f)
		dstName := getDstName(inst, f)
		if srcName == "" || dstName == "" {
			return fmt.Errorf("Unexpected copy: %q -> %q", srcName, dstName)
		}

		if err := sem.Acquire(ctx, 1); err != nil {
			return err
		}
		g.Go(func() error {
			defer sem.Release(1)
			err := utils.RetryWithExpBackoff(3, 200*time.Millisecond, func() error {
				_, err := c.ObjectCopy(src, srcName, dst, dstName, nil)
				return err
			})
			if err != nil {
				log.Warningf("Cannot copy file from %s %s to %s %s: %s",
					src, srcName, dst, dstName, err)
			}
			return err
		})

		// Copy the thumbnails
		if f.Class == "image" {
			srcSmall, srcMedium, srcLarge := getThumbsSrcNames(inst, f)
			dstSmall, dstMedium, dstLarge := getThumbsDstNames(inst, f)
			if err := sem.Acquire(ctx, 1); err != nil {
				return err
			}
			g.Go(func() error {
				defer sem.Release(1)
				_, err := c.ObjectCopy(thumbsContainer, srcSmall, dst, dstSmall, nil)
				if err != nil {
					log.Infof("Cannot copy thumbnail small from %s %s to %s %s: %s",
						thumbsContainer, srcSmall, dst, dstSmall, err)
				}
				_, err = c.ObjectCopy(thumbsContainer, srcMedium, dst, dstMedium, nil)
				if err != nil {
					log.Infof("Cannot copy thumbnail medium from %s %s to %s %s: %s",
						thumbsContainer, srcMedium, dst, dstMedium, err)
				}
				_, err = c.ObjectCopy(thumbsContainer, srcLarge, dst, dstLarge, nil)
				if err != nil {
					log.Infof("Cannot copy thumbnail large from %s %s to %s %s: %s",
						thumbsContainer, srcLarge, dst, dstLarge, err)
				}
				return nil
			})
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}
	return errw
}

func getSrcName(inst *instance.Instance, f *vfs.FileDoc) string {
	srcName := ""
	switch inst.SwiftLayout {
	case 0: // layout v1
		srcName = f.DirID + "/" + f.DocName
	case 1: // layout v2
		srcName = vfsswift.MakeObjectName(f.DocID)
	}
	return srcName
}

// XXX the f FileDoc can be modified to add an InternalID
func getDstName(inst *instance.Instance, f *vfs.FileDoc) string {
	if f.InternalID == "" {
		old := f.Clone().(*vfs.FileDoc)
		f.InternalID = vfsswift.NewInternalID()
		if err := couchdb.UpdateDocWithOld(inst, f, old); err != nil {
			return ""
		}
	}
	return vfsswift.MakeObjectNameV3(f.DocID, f.InternalID)
}

func getThumbsSrcNames(inst *instance.Instance, f *vfs.FileDoc) (string, string, string) {
	var small, medium, large string
	switch inst.SwiftLayout {
	case 0: // layout v1
		small = fmt.Sprintf("thumbs/%s-small", f.DocID)
		medium = fmt.Sprintf("thumbs/%s-medium", f.DocID)
		large = fmt.Sprintf("thumbs/%s-large", f.DocID)
	case 1: // layout v2
		obj := vfsswift.MakeObjectName(f.DocID)
		small = fmt.Sprintf("thumbs/%s-small", obj)
		medium = fmt.Sprintf("thumbs/%s-medium", obj)
		large = fmt.Sprintf("thumbs/%s-large", obj)
	}
	return small, medium, large
}

func getThumbsDstNames(inst *instance.Instance, f *vfs.FileDoc) (string, string, string) {
	obj := vfsswift.MakeObjectName(f.DocID)
	small := fmt.Sprintf("thumbs/%s-small", obj)
	medium := fmt.Sprintf("thumbs/%s-medium", obj)
	large := fmt.Sprintf("thumbs/%s-large", obj)
	return small, medium, large
}
