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
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/stack"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var flagAllowRoot bool
var flagAppdirs []string
var flagDevMode bool

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

		if flagDevMode {
			build.BuildMode = build.ModeDev
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

		if !build.IsDevRelease() {
			adminSecretFile := config.GetConfig().AdminSecretFileName
			if _, err := config.FindConfigFile(adminSecretFile); err != nil {
				return err
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

		fmt.Println("Ready and waiting for connections:")
		servers.Start()

		group := utils.NewGroupShutdown(servers, processes)

		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, os.Interrupt)

		select {
		case err := <-servers.Wait():
			return err
		case <-sigs:
			fmt.Println("\nReceived interrupt signal:")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel() // make gometalinter happy
			if err := group.Shutdown(ctx); err != nil {
				return err
			}
			fmt.Println("All settled, bye bye !")
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
		Path:   path.Join(filepath.ToSlash(binDir), DefaultStorageDir),
	}
	flags.String("fs-url", defaultFsURL.String(), "filesystem url")
	checkNoErr(viper.BindPFlag("fs.url", flags.Lookup("fs-url")))

	flags.String("couchdb-url", "http://localhost:5984/", "CouchDB URL")
	checkNoErr(viper.BindPFlag("couchdb.url", flags.Lookup("couchdb-url")))

	flags.String("lock-url", "", "URL for the locks, redis or in-memory")
	checkNoErr(viper.BindPFlag("lock.url", flags.Lookup("lock-url")))

	flags.String("sessions-url", "", "URL for the sessions storage, redis or in-memory")
	checkNoErr(viper.BindPFlag("sessions.url", flags.Lookup("sessions-url")))

	flags.String("downloads-url", "", "URL for the download secret storage, redis or in-memory")
	checkNoErr(viper.BindPFlag("downloads.url", flags.Lookup("downloads-url")))

	flags.String("jobs-url", "", "URL for the jobs system synchronization, redis or in-memory")
	checkNoErr(viper.BindPFlag("jobs.url", flags.Lookup("jobs-url")))

	flags.String("konnectors-cmd", "", "konnectors command to be executed")
	checkNoErr(viper.BindPFlag("konnectors.cmd", flags.Lookup("konnectors-cmd")))

	flags.String("konnectors-oauthstate", "", "URL for the storage of OAuth state for konnectors, redis or in-memory")
	checkNoErr(viper.BindPFlag("konnectors.oauthstate", flags.Lookup("konnectors-oauthstate")))

	flags.String("realtime-url", "", "URL for realtime in the browser via webocket, redis or in-memory")
	checkNoErr(viper.BindPFlag("realtime.url", flags.Lookup("realtime-url")))

	flags.String("rate-limiting-url", "", "URL for rate-limiting counters, redis or in-memory")
	checkNoErr(viper.BindPFlag("rate_limiting.url", flags.Lookup("rate-limiting-url")))

	flags.String("log-level", "info", "define the log level")
	checkNoErr(viper.BindPFlag("log.level", flags.Lookup("log-level")))

	flags.Bool("log-syslog", false, "use the local syslog for logging")
	checkNoErr(viper.BindPFlag("log.syslog", flags.Lookup("log-syslog")))

	flags.String("hooks", ".", "define the directory used for hook scripts")
	checkNoErr(viper.BindPFlag("hooks", flags.Lookup("hooks")))

	flags.String("geodb", ".", "define the location of the database for IP -> City lookups")
	checkNoErr(viper.BindPFlag("geodb", flags.Lookup("geodb")))

	flags.String("mail-noreply-address", "", "mail address used for sending mail as a noreply (forgot passwords for example)")
	checkNoErr(viper.BindPFlag("mail.noreply_address", flags.Lookup("mail-noreply-address")))

	flags.String("mail-noreply-name", "", "mail name used for sending mail as a noreply (forgot passwords for example)")
	checkNoErr(viper.BindPFlag("mail.noreply_name", flags.Lookup("mail-noreply-name")))

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

	flags.String("password-reset-interval", "15m", "minimal duration between two password reset")
	checkNoErr(viper.BindPFlag("password_reset_interval", flags.Lookup("password-reset-interval")))

	flags.BoolVar(&flagDevMode, "dev", false, "Allow to run in dev mode for a prod release (disabled by default)")
	flags.BoolVar(&flagAllowRoot, "allow-root", false, "Allow to start as root (disabled by default)")
	flags.StringSliceVar(&flagAppdirs, "appdir", nil, "Mount a directory as the 'app' application")

	flags.Bool("disable-csp", false, "Disable the Content Security Policy (only available for development)")
	checkNoErr(viper.BindPFlag("disable_csp", flags.Lookup("disable-csp")))

	flags.String("csp-whitelist", "", "Whitelisted domains for the default allowed origins of the Content Secury Policy")
	checkNoErr(viper.BindPFlag("csp_whitelist", flags.Lookup("csp-whitelist")))

	RootCmd.AddCommand(serveCmd)
}
