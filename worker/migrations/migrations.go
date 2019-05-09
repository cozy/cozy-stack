package migrations

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs/vfsswift"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/swift"
	multierror "github.com/hashicorp/go-multierror"
)

const swiftV1ContainerPrefixCozy = "cozy-"
const swiftV1ContainerPrefixData = "data-"
const swiftV2ContainerPrefixCozy = "cozy-v2-"
const swiftV2ContainerPrefixData = "data-v2"
const versionSuffix = "-version"
const dirContentType = "directory"

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "migrations",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		WorkerFunc:   worker,
		WorkerCommit: commit,
	})
}

const swiftV1ToV2 = "swift-v1-to-v2"

type message struct {
	Type    string `json:"type"`
	Cluster int    `json:"cluster"`
}

func worker(ctx *job.WorkerContext) error {
	var msg message
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}

	switch msg.Type {
	case swiftV1ToV2:
		return migrateSwiftV1ToV2(ctx.Instance.Domain)
	default:
		return fmt.Errorf("unknown migration type %q", msg.Type)
	}
}

func commit(ctx *job.WorkerContext, err error) error {
	if err != nil {
		return nil
	}

	var msg message
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}

	switch msg.Type {
	case swiftV1ToV2:
		return commitSwiftV1ToV2(ctx.Instance.Domain, msg.Cluster)
	default:
		return fmt.Errorf("unknown migration type %q", msg.Type)
	}
}

type object struct {
	obj          swift.Object
	containerSrc string
	containerDst string
}

func migrateSwiftV1ToV2(domain string) error {
	c := config.GetSwiftConnection()
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}
	if inst.SwiftCluster > 0 {
		return nil
	}

	containerV1 := swiftV1ContainerPrefixCozy + domain
	containerV2 := swiftV2ContainerPrefixCozy + domain

	// container containing thumbnails
	containerV1Data := swiftV1ContainerPrefixData + domain
	containerV2Data := swiftV2ContainerPrefixData + domain

	err = c.VersionContainerCreate(containerV2, containerV2+versionSuffix)
	if err != nil {
		return err
	}

	objc := make(chan object)
	errc := make(chan error)

	go func() {
		errc <- readObjects(c, objc, containerV1, containerV2)
		errc <- readObjects(c, objc, containerV1Data, containerV2Data)
		close(objc)
	}()

	const N = 4

	for i := 0; i < N; i++ {
		go copyObjects(c, inst, objc, errc)
	}

	var errm error
	done := N
	for {
		err := <-errc
		if err != nil {
			errm = multierror.Append(errm, err)
		} else {
			done--
		}
		if done == 0 {
			break
		}
	}

	return errm
}

func commitSwiftV1ToV2(domain string, swiftCluster int) error {
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}

	c := config.GetSwiftConnection()

	containerName := swiftV1ContainerPrefixCozy + domain
	containerMeta := &swift.Metadata{"cozy-v1-migrated": "1"}
	err = c.ContainerUpdate(containerName, containerMeta.ContainerHeaders())
	if err != nil {
		return err
	}

	return lifecycle.Patch(inst, &lifecycle.Options{SwiftCluster: swiftCluster})
}

func readObjects(c *swift.Connection, objc chan object,
	containerSrc, containerDst string) error {
	return c.ObjectsWalk(containerSrc, nil, func(opts *swift.ObjectsOpts) (interface{}, error) {
		objs, err := c.Objects(containerSrc, opts)
		if err != nil {
			return nil, err
		}
		for _, obj := range objs {
			objc <- object{
				obj:          obj,
				containerSrc: containerSrc,
				containerDst: containerDst,
			}
		}
		return objs, err
	})
}

func copyObjects(c *swift.Connection, db prefixer.Prefixer,
	objc chan object,
	errc chan error) {

	copyBuffer := make([]byte, 128*1024)

	for obj := range objc {
		var err error
		containerSrc := obj.containerSrc
		containerDst := obj.containerDst
		switch {
		case strings.HasPrefix(containerSrc, swiftV1ContainerPrefixCozy):
			err = copyFileDataObject(c, db, containerSrc, containerDst, obj.obj, copyBuffer)
		case strings.HasPrefix(containerSrc, swiftV1ContainerPrefixData):
			err = copyThumbnailDataObject(c, db, containerSrc, containerDst, obj.obj, copyBuffer)
		}
		if err != nil {
			errc <- err
		}
	}

	errc <- nil
}

func copyFileDataObject(c *swift.Connection, db prefixer.Prefixer,
	containerSrc, containerDst string,
	objSrc swift.Object,
	copyBuffer []byte) error {
	if objSrc.ContentType == dirContentType {
		return nil
	}
	dirID, name, ok := splitV2ObjectName(objSrc.Name)
	if !ok {
		return nil
	}
	var res couchdb.ViewResponse
	err := couchdb.ExecView(db, couchdb.FilesByParentView, &couchdb.ViewRequest{
		Key:         []string{dirID, consts.FileType, name},
		IncludeDocs: false,
	}, &res)
	if err != nil {
		return err
	}
	if len(res.Rows) == 0 {
		return os.ErrNotExist
	}
	objNameDst := vfsswift.MakeObjectName(res.Rows[0].ID)
	return copyObject(c, db, containerSrc, containerDst, objSrc, objNameDst, copyBuffer)
}

func copyThumbnailDataObject(c *swift.Connection, db prefixer.Prefixer,
	containerSrc, containerDst string,
	objSrc swift.Object,
	copyBuffer []byte) error {

	split := strings.SplitN(strings.TrimPrefix(objSrc.Name, "thumbs/"), "-", 2)
	if len(split) != 2 {
		return nil
	}
	objNameDst := "thumbs/" + vfsswift.MakeObjectName(split[0]) + "-" + split[1]
	return copyObject(c, db, containerSrc, containerDst, objSrc, objNameDst, copyBuffer)
}

func copyObject(c *swift.Connection, db prefixer.Prefixer,
	containerSrc, containerDst string,
	objSrc swift.Object, objNameDst string,
	copyBuffer []byte) (err error) {
	infosDst, _, err := c.Object(containerDst, objNameDst)
	if err == nil && infosDst.Hash == objSrc.Hash {
		return nil
	}
	if err != swift.ObjectNotFound {
		return err
	}

	src, h, err := c.ObjectOpen(containerSrc, objSrc.Name, false, nil)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := c.ObjectCreate(containerDst, objNameDst, true, objSrc.Hash, objSrc.ContentType, h)
	if err != nil {
		return err
	}
	defer func() {
		if errc := dst.Close(); errc != nil && err == nil {
			err = errc
		}
	}()

	_, err = io.CopyBuffer(dst, src, copyBuffer)
	return err
}

func splitV2ObjectName(objName string) (dirID string, name string, ok bool) {
	split := strings.SplitN(objName, "/", 2)
	if len(split) != 2 {
		return
	}
	dirID, name = split[0], split[1]
	ok = true
	return
}
