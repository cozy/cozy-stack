package cmd

import (
	"errors"
	"os"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web"
	"github.com/spf13/cobra"
)

var flagAllowRoot bool
var flagAppdir string

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
example), you can use the --appsdir flag like this:

	$ cozy-stack serve --appsdir appone:/path/to/app_one,apptwo:/path/to/app_two
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !flagAllowRoot && os.Getuid() == 0 {
			log.Errorf("Use --allow-root if you really want to start with the root user")
			return errors.New("Starting cozy-stack serve as root not allowed")
		}
		if err := instance.StartJobs(); err != nil {
			return err
		}
		if flagAppdir != "" {
			apps := make(map[string]string)
			for _, app := range strings.Split(flagAppdir, ",") {
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
		return web.ListenAndServe()
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
	serveCmd.Flags().BoolVar(&flagAllowRoot, "allow-root", false, "Allow to start as root (disabled by default)")
	serveCmd.Flags().StringVar(&flagAppdir, "appdir", "", "Mount a directory as the 'app' application")
}
