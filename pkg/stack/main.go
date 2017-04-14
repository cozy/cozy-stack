package stack

import (
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsswift"
)

// Start is used to initialize all the
func Start() error {
	if err := jobs.StartSystem(); err != nil {
		return err
	}

	// Init the main global connection to the swift server
	fsURL := config.FsURL()
	if fsURL.Scheme == "swift" {
		if err := vfsswift.InitConnection(fsURL); err != nil {
			return err
		}
	}

	return nil
}
