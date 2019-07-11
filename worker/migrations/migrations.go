package migrations

import (
	"fmt"
	"runtime"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/model/vfs/vfsswift"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/swift"
	multierror "github.com/hashicorp/go-multierror"
)

const (
	swiftV1ToV2 = "swift-v1-to-v2"
	toSwiftV3   = "to-swift-v3"

	swiftV1ContainerPrefixCozy = "cozy-"
	swiftV1ContainerPrefixData = "data-"
	swiftV2ContainerPrefixCozy = "cozy-v2-"
	swiftV2ContainerPrefixData = "data-v2-"
	swiftV3ContainerPrefix     = "cozy-v3-"
)

// maxSimultaneousCalls is the maximal number of simultaneous calls to Swift
// made by a single migration.
const maxSimultaneousCalls = 8

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "migrations",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		WorkerFunc:   worker,
		WorkerCommit: commit,
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

	switch msg.Type {
	case toSwiftV3:
		return migrateToSwiftV3(ctx.Instance.Domain)
	case swiftV1ToV2:
		return fmt.Errorf("this migration type is no longer supported")
	default:
		return fmt.Errorf("unknown migration type %q", msg.Type)
	}
}

func commit(ctx *job.WorkerContext, err error) error {
	return err
}

func migrateToSwiftV3(domain string) error {
	c := config.GetSwiftConnection()
	inst, err := instance.GetFromCouch(domain)
	if err != nil {
		return err
	}

	var srcContainer string
	switch inst.SwiftLayout {
	case 0: // layout v1
		srcContainer = swiftV1ContainerPrefixCozy + domain
	case 1: // layout v2
		srcContainer = swiftV2ContainerPrefixCozy + inst.DBPrefix()
	case 2: // layout v3
		return nil // Nothing to do!
	default:
		return instance.ErrInvalidSwiftLayout
	}

	mutex := lock.ReadWrite(inst, "vfs")
	if err = mutex.Lock(); err != nil {
		return err
	}
	defer mutex.Unlock()

	dstContainer := swiftV3ContainerPrefix + inst.DBPrefix()
	if err = c.ContainerCreate(dstContainer, nil); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = vfsswift.DeleteContainer(c, dstContainer)
		}
	}()

	if err = copyTheFilesToSwiftV3(inst, c, srcContainer, dstContainer); err != nil {
		return err
	}

	vfs := inst.VFS()
	meta := &swift.Metadata{"cozy-migrated-from-v1": "1"}
	_ = c.ContainerUpdate(srcContainer, meta.ContainerHeaders())
	if err = lifecycle.Patch(inst, &lifecycle.Options{SwiftLayout: 2}); err != nil {
		return err
	}
	_ = vfs.Delete()
	return nil
}

func copyTheFilesToSwiftV3(inst *instance.Instance, c *swift.Connection, src, dst string) error {
	nb := 0
	ch := make(chan error)

	var thumbsContainer string
	switch inst.SwiftLayout {
	case 0: // layout v1
		thumbsContainer = swiftV1ContainerPrefixData + inst.Domain
	case 1: // layout v2
		thumbsContainer = swiftV2ContainerPrefixData + inst.DBPrefix()
	default:
		return instance.ErrInvalidSwiftLayout
	}

	// Use a system of tokens to limit the number of simultaneous calls to
	// Swift: only a goroutine that has a token can make a call.
	tokens := make(chan int, maxSimultaneousCalls)
	for k := 0; k < maxSimultaneousCalls; k++ {
		tokens <- k
	}

	fs := inst.VFS()
	errm := vfs.Walk(fs, "/", func(fullpath string, d *vfs.DirDoc, f *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}
		if f == nil {
			return nil
		}

		srcName := getSrcName(inst, f, fullpath)
		dstName := getDstName(inst, f)
		if srcName == "" || dstName == "" {
			return fmt.Errorf("Unexpected copy: %q -> %q", srcName, dstName)
		}

		nb++
		go func() {
			k := <-tokens
			_, err := c.ObjectCopy(src, srcName, dst, dstName, nil)
			ch <- err
			tokens <- k
		}()

		// Copy the thumbnails
		if f.Class == "image" {
			srcSmall, srcMedium, srcLarge := getThumbsSrcNames(inst, f)
			dstSmall, dstMedium, dstLarge := getThumbsDstNames(inst, f)
			nb += 3
			go func() {
				k := <-tokens
				_, err := c.ObjectCopy(thumbsContainer, srcSmall, dst, dstSmall, nil)
				ch <- err
				_, err = c.ObjectCopy(thumbsContainer, srcMedium, dst, dstMedium, nil)
				ch <- err
				_, err = c.ObjectCopy(thumbsContainer, srcLarge, dst, dstLarge, nil)
				ch <- err
				tokens <- k
			}()
		}
		return nil
	})

	for i := 0; i < nb; i++ {
		if err := <-ch; err != nil {
			errm = multierror.Append(errm, err)
		}
	}
	// Get back the tokens to ensure that each goroutine can finish.
	for k := 0; k < maxSimultaneousCalls; k++ {
		<-tokens
	}
	return errm
}

func getSrcName(inst *instance.Instance, f *vfs.FileDoc, fullpath string) string {
	srcName := ""
	switch inst.SwiftLayout {
	case 0: // layout v1
		srcName = fullpath
	case 1: // layout v2
		srcName = vfsswift.MakeObjectName(f.DocID)
	}
	return srcName
}

func getDstName(inst *instance.Instance, f *vfs.FileDoc) string {
	if f.InternalID == "" {
		f.InternalID = vfsswift.NewInternalID()
		if err := couchdb.UpdateDoc(inst, f); err != nil {
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
