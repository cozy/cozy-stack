package stack

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/utils"

	"github.com/google/gops/agent"
	"github.com/sirupsen/logrus"
)

// Options can be used to give options when starting the stack.
type Options int

const (
	// NoGops option can be used to disable gops support
	NoGops Options = iota + 1
	// NoDynAssets option can be used to initialize the dynamic assets
	NoDynAssets
)

func hasOptions(needle Options, haystack []Options) bool {
	for _, opt := range haystack {
		if opt == needle {
			return true
		}
	}
	return false
}

type gopAgent struct{}

func (g gopAgent) Shutdown(ctx context.Context) error {
	fmt.Print("  shutting down gops...")
	agent.Close()
	fmt.Println("ok.")
	return nil
}

// Start is used to initialize all the
func Start(opts ...Options) (processes utils.Shutdowner, err error) {
	if build.IsDevRelease() {
		fmt.Print(`                           !! DEVELOPMENT RELEASE !!
You are running a development release which may deactivate some very important
security features. Please do not use this binary as your production server.

`)
	}

	var shutdowners []utils.Shutdowner

	if !hasOptions(NoGops, opts) {
		err = agent.Listen(agent.Options{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error on gops agent: %s\n", err)
		}
		shutdowners = append(shutdowners, gopAgent{})
	}

	if err = config.MakeVault(config.GetConfig()); err != nil {
		return
	}

	// Check that we can properly reach CouchDB.
	attempts := 8
	attemptsSpacing := 1 * time.Second
	for i := 0; i < attempts; i++ {
		_, err = couchdb.CheckStatus()
		if err == nil {
			break
		}
		err = fmt.Errorf("Could not reach Couchdb 2 database: %s", err.Error())
		if i < attempts-1 {
			logrus.Warnf("%s, retrying in %v", err, attemptsSpacing)
			time.Sleep(attemptsSpacing)
		}
	}
	if err != nil {
		return
	}
	if err = couchdb.InitGlobalDB(); err != nil {
		return
	}

	// Init the main global connection to the swift server
	if err = config.InitDefaultSwiftConnection(); err != nil {
		return
	}

	workersList, err := job.GetWorkersList()
	if err != nil {
		return
	}

	var broker job.Broker
	var schder job.Scheduler
	jobsConfig := config.GetConfig().Jobs
	if cli := jobsConfig.Client(); cli != nil {
		broker = job.NewRedisBroker(cli)
		schder = job.NewRedisScheduler(cli)
	} else {
		broker = job.NewMemBroker()
		schder = job.NewMemScheduler()
	}

	if err = job.SystemStart(broker, schder, workersList); err != nil {
		return
	}
	shutdowners = append(shutdowners, job.System())

	// Initialize the dynamic assets FS. Can be OsFs, MemFs or Swift
	if !hasOptions(NoDynAssets, opts) {
		err = dynamic.InitDynamicAssetFS()
		if err != nil {
			return nil, err
		}
	}

	sessionSweeper := session.SweepLoginRegistrations()
	shutdowners = append(shutdowners, sessionSweeper)

	// Global shutdowner that composes all the running processes of the stack
	processes = utils.NewGroupShutdown(shutdowners...)
	return
}
