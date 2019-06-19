package vfsswift

import (
	"errors"

	"github.com/cozy/cozy-stack/model/vfs"
)

func (sfs *swiftVFSV3) Fsck(accumulate func(log *vfs.FsckLog)) (err error) {
	return errors.New("Not implemented") // TODO
}
