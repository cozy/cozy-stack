package cmd

import (
	"errors"
	"os"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/web"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var flagNoAdmin bool
var flagAllowRoot bool
var flagAppdirs []string

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the stack and listens for HTTP calls",
	Long: `Starts the stack and listens for HTTP calls
It will accept HTTP requests on localhost:8080 by default.
Use the --port and --host flags to change the listening option.

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
		if err := stack.Start(); err != nil {
			return err
		}
		if len(flagAppdirs) > 0 {
			apps := make(map[string]string)
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
			return web.ListenAndServeWithAppDir(apps)
		}
		return web.ListenAndServe(flagNoAdmin)
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

	defaultFsUrl := &url.URL{
		Scheme: "file",
		Path: path.Join(filepath.ToSlash(binDir), DefaultStorageDir),
	}
	flags.String("fs-url", defaultFsUrl.String(), "filesystem url")
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

	flags.String("log-level", "info", "define the log level")
	checkNoErr(viper.BindPFlag("log.level", flags.Lookup("log-level")))

	flags.Bool("log-syslog", false, "use the local syslog for logging")
	checkNoErr(viper.BindPFlag("log.syslog", flags.Lookup("log-syslog")))

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
	serveCmd.Flags().BoolVar(&flagNoAdmin, "no-admin", false, "Start without the admin interface")
	serveCmd.Flags().BoolVar(&flagAllowRoot, "allow-root", false, "Allow to start as root (disabled by default)")
	serveCmd.Flags().StringSliceVar(&flagAppdirs, "appdir", nil, "Mount a directory as the 'app' application")
}
