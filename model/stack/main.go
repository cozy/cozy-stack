package stack

import (
	"context"
	"fmt"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	"os"

	"github.com/cozy/cozy-stack/model/cloudery"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/settings"
	"github.com/cozy/cozy-stack/model/token"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/emailer"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/google/gops/agent"
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

type Services struct {
	Emailer  emailer.Emailer
	Settings settings.Service
}

// Start is used to initialize all the
func Start(opts ...Options) (utils.Shutdowner, *Services, error) {
	if build.IsDevRelease() {
		fmt.Print(`                           !! DEVELOPMENT RELEASE !!
You are running a development release which may deactivate some very important
security features. Please do not use this binary as your production server.

`)
	}

	var shutdowners []utils.Shutdowner

	ctx := context.Background()

	if !hasOptions(NoGops, opts) {
		err := agent.Listen(agent.Options{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error on gops agent: %s\n", err)
		}
		shutdowners = append(shutdowners, gopAgent{})
	}

	if err := couchdb.InitGlobalDB(ctx); err != nil {
		return nil, nil, fmt.Errorf("failed to init the global db: %w", err)
	}

	// Init the main global connection to the swift server
	if err := config.InitDefaultSwiftConnection(); err != nil {
		return nil, nil, fmt.Errorf("failed to init the swift connection: %w", err)
	}

	workersList, err := job.GetWorkersList()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the workers list: %w", err)
	}

	var broker job.Broker
	var schder job.Scheduler
	jobsConfig := config.GetConfig().Jobs
	if jobsConfig.Client != nil {
		broker = job.NewRedisBroker(jobsConfig.Client)
		schder = job.NewRedisScheduler(jobsConfig.Client)
	} else {
		broker = job.NewMemBroker()
		schder = job.NewMemScheduler()
	}

	if err = job.SystemStart(broker, schder, workersList); err != nil {
		return nil, nil, fmt.Errorf("failed to start the jobs: %w", err)
	}
	shutdowners = append(shutdowners, job.System())

	tokenSvc := token.NewService(config.GetConfig().CacheStorage)
	emailerSvc := emailer.Init()
	instanceSvc := instance.Init()
	clouderySvc := cloudery.Init(config.GetConfig().Clouderies)

	services := Services{
		Emailer:  emailerSvc,
		Settings: settings.Init(emailerSvc, instanceSvc, tokenSvc, clouderySvc),
	}

	// Initialize the dynamic assets FS. Can be OsFs, MemFs or Swift
	if !hasOptions(NoDynAssets, opts) {
		err = dynamic.InitDynamicAssetFS(config.FsURL().String())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to init the dynamic asset fs: %w", err)
		}
	}

	// Start RabbitMQ manager if enabled in configuration
	if cfg := config.GetConfig().RabbitMQ; cfg.Enabled {
		shutdowner, err := rabbitmq.Start(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to start rabbitmq manager: %w", err)
		}
		shutdowners = append(shutdowners, shutdowner)
	}

	// Global shutdowner that composes all the running processes of the stack
	processes := utils.NewGroupShutdown(shutdowners...)

	return processes, &services, nil
}
