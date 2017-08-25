package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var flagAllowRoot bool
var flagAppdirs []string

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the stack and listens for HTTP calls",
	Long: `Starts the stack and listens for HTTP calls
It will accept HTTP requests on localhost:8080 by default.
Use the --port and --host flags to change the listening option.

The SIGINT signal will trigger a graceful stop of cozy-stack: it will wait that
current HTTP requests and jobs are finished (in a limit of 2 minutes) before
exiting.

If you are the developer of a client-side app, you can use --appdir
to mount a directory as the application with the 'app' slug.
`,
	Example: `The most often, this command is used in its simple form:

	$ cozy-stack serve

But if you want to develop two apps in local (to test their interactions for
example), you can use the --appdir flag like this:

	$ cozy-stack serve --appdir appone:/path/to/app_one,apptwo:/path/to/app_two
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !flagAllowRoot && os.Getuid() == 0 {
			errPrintfln("Use --allow-root if you really want to start with the root user")
			return errors.New("Starting cozy-stack serve as root not allowed")
		}
		var apps map[string]string
		if len(flagAppdirs) > 0 {
			apps = make(map[string]string)
			for _, app := range flagAppdirs {
				parts := strings.Split(app, ":")
				switch len(parts) {
				case 1:
					apps["app"] = parts[0]
				case 2:
					apps[parts[0]] = parts[1]
				default:
					return errors.New("Invalid appdir value")
				}
			}
		}

		processes, err := stack.Start()
		if err != nil {
			return err
		}

		var servers *web.Servers
		if apps != nil {
			servers, err = web.ListenAndServeWithAppDir(apps)
		} else {
			servers, err = web.ListenAndServe()
		}
		if err != nil {
			return err
		}
		servers.Start()

		group := utils.NewGroupShutdown(servers, processes)

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, os.Interrupt)

		select {
		case err := <-servers.Wait():
			return err
		case <-sigs:
			fmt.Println("\nshutdown started")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel() // make gometalinter happy
			if err := group.Shutdown(ctx); err != nil {
				return err
			}
			fmt.Println("all settled, bye bye !")
			return nil
		}
	},
}

func init() {
	binDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err)
	}

	flags := serveCmd.PersistentFlags()
	flags.String("subdomains", "nested", "how to structure the subdomains for apps (can be nested or flat)")
	checkNoErr(viper.BindPFlag("subdomains", flags.Lookup("subdomains")))

	flags.String("assets", "", "path to the directory with the assets (use the packed assets by default)")
	checkNoErr(viper.BindPFlag("assets", flags.Lookup("assets")))

	flags.String("doctypes", "", "path to the directory with the doctypes (for developing/testing a remote doctype)")
	checkNoErr(viper.BindPFlag("doctypes", flags.Lookup("doctypes")))

	defaultFsURL := &url.URL{
		Scheme: "file",
		Path: path.Join(filepath.ToSlash(binDir), DefaultStorageDir),
	}
	flags.String("fs-url", defaultFsURL.String(), "filesystem url")
	checkNoErr(viper.BindPFlag("fs.url", flags.Lookup("fs-url")))

	flags.String("couchdb-url", "http://localhost:5984/", "CouchDB URL")
	checkNoErr(viper.BindPFlag("couchdb.url", flags.Lookup("couchdb-url")))

	flags.String("cache-url", "", "URL for the cache, redis or in-memory")
	checkNoErr(viper.BindPFlag("cache.url", flags.Lookup("cache-url")))

	flags.String("lock-url", "", "URL for the locks, redis or in-memory")
	checkNoErr(viper.BindPFlag("lock.url", flags.Lookup("lock-url")))

	flags.String("sessions-url", "", "URL for the sessions storage, redis or in-memory")
	checkNoErr(viper.BindPFlag("sessions.url", flags.Lookup("sessions-url")))

	flags.String("downloads-url", "", "URL for the download secret storage, redis or in-memory")
	checkNoErr(viper.BindPFlag("downloads.url", flags.Lookup("downloads-url")))

	flags.Int("jobs-workers", runtime.NumCPU(), "Number of parallel workers (0 to disable the processing of jobs)")
	checkNoErr(viper.BindPFlag("jobs.workers", flags.Lookup("jobs-workers")))

	flags.String("jobs-url", "", "URL for the jobs system synchronization, redis or in-memory")
	checkNoErr(viper.BindPFlag("jobs.url", flags.Lookup("jobs-url")))

	flags.String("konnectors-cmd", "", "konnectors command to be executed")
	checkNoErr(viper.BindPFlag("konnectors.cmd", flags.Lookup("konnectors-cmd")))

	flags.String("konnectors-oauthstate", "", "URL for the storage of OAuth state for konnectors, redis or in-memory")
	checkNoErr(viper.BindPFlag("konnectors.oauthstate", flags.Lookup("konnectors-oauthstate")))

	flags.String("realtime-url", "", "URL for realtime in the browser via webocket, redis or in-memory")
	checkNoErr(viper.BindPFlag("realtime.url", flags.Lookup("realtime-url")))

	flags.String("log-level", "info", "define the log level")
	checkNoErr(viper.BindPFlag("log.level", flags.Lookup("log-level")))

	flags.Bool("log-syslog", false, "use the local syslog for logging")
	checkNoErr(viper.BindPFlag("log.syslog", flags.Lookup("log-syslog")))

	flags.String("hooks", ".", "define the directory used for hook scripts")
	checkNoErr(viper.BindPFlag("hooks", flags.Lookup("hooks")))

	flags.String("mail-noreply-address", "", "mail address used for sending mail as a noreply (forgot passwords for example)")
	checkNoErr(viper.BindPFlag("mail.noreply_address", flags.Lookup("mail-noreply-address")))

	flags.String("mail-host", "localhost", "mail smtp host")
	checkNoErr(viper.BindPFlag("mail.host", flags.Lookup("mail-host")))

	flags.Int("mail-port", 465, "mail smtp port")
	checkNoErr(viper.BindPFlag("mail.port", flags.Lookup("mail-port")))

	flags.String("mail-username", "", "mail smtp username")
	checkNoErr(viper.BindPFlag("mail.username", flags.Lookup("mail-username")))

	flags.String("mail-password", "", "mail smtp password")
	checkNoErr(viper.BindPFlag("mail.password", flags.Lookup("mail-password")))

	flags.Bool("mail-disable-tls", false, "disable smtp over tls")
	checkNoErr(viper.BindPFlag("mail.disable_tls", flags.Lookup("mail-disable-tls")))

	RootCmd.AddCommand(serveCmd)
	serveCmd.Flags().BoolVar(&flagAllowRoot, "allow-root", false, "Allow to start as root (disabled by default)")
	serveCmd.Flags().StringSliceVar(&flagAppdirs, "appdir", nil, "Mount a directory as the 'app' application")
}
