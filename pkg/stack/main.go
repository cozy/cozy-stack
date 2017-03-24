package stack

import (
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs/vfsswift"
)

// Start is used to initialize all the
func Start() error {
	// StartJobs is used to start the job system for all the instances.
	// TODO: on distributed stacks, we should not have to iterate over all
	// instances on each startup
	instances, err := instance.List()
	if err != nil && !couchdb.IsNoDatabaseError(err) {
		return err
	}
	for _, in := range instances {
		if err := in.StartJobSystem(); err != nil {
			return err
		}
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
