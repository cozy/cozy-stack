package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the stack and listens for HTTP calls",
	Long: `Starts the stack and listens for HTTP calls
It will accept HTTP requests on localhost:8080 by default.
Use the --port and --host flags to change the listening option.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// The serve method starts all the jobs systems associated with the created
		// instances.
		//
		// TODO: on distributed stacks, we should not have to iterate over all
		// instances on each startup
		ins, err := instance.List()
		if err != nil && !couchdb.IsNoDatabaseError(err) {
			return err
		}
		for _, in := range ins {
			if err := in.StartJobSystem(); err != nil {
				return err
			}
		}
		return web.ListenAndServe()
	},
}

func init() {
	binDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err)
	}

	flags := serveCmd.PersistentFlags()
	flags.String("host", "localhost", "server host")
	checkNoErr(viper.BindPFlag("host", flags.Lookup("host")))

	flags.IntP("port", "p", 8080, "server port")
	checkNoErr(viper.BindPFlag("port", flags.Lookup("port")))

	flags.String("subdomains", "nested", "how to structure the subdomains for apps (can be nested or flat)")
	checkNoErr(viper.BindPFlag("subdomains", flags.Lookup("subdomains")))

	flags.String("assets", "", "path to the directory with the assets (use the packed assets by default)")
	checkNoErr(viper.BindPFlag("assets", flags.Lookup("assets")))

	flags.String("admin-host", "localhost", "administration server host")
	checkNoErr(viper.BindPFlag("admin.host", flags.Lookup("admin-host")))

	flags.Int("admin-port", 6060, "administration server port")
	checkNoErr(viper.BindPFlag("admin.port", flags.Lookup("admin-port")))

	flags.String("fs-url", fmt.Sprintf("file://localhost%s/%s", binDir, DefaultStorageDir), "filesystem url")
	checkNoErr(viper.BindPFlag("fs.url", flags.Lookup("fs-url")))

	flags.String("couchdb-host", "localhost", "couchdbdb host")
	checkNoErr(viper.BindPFlag("couchdb.host", flags.Lookup("couchdb-host")))

	flags.Int("couchdb-port", 5984, "couchdbdb port")
	checkNoErr(viper.BindPFlag("couchdb.port", flags.Lookup("couchdb-port")))

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
}
