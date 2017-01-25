package cmd

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
)

var flagLocale string
var flagTimezone string
var flagEmail string
var flagApps []string
var flagDev bool

func validDomain(domain string) bool {
	return !strings.ContainsAny(domain, " /?#@\t\r\n")
}

// instanceCmdGroup represents the instances command
var instanceCmdGroup = &cobra.Command{
	Use:   "instances [command]",
	Short: "Manage instances of a stack",
	Long: `
cozy-stack instances allows to manage the instances of this stack

An instance is a logical space owned by one user and identified by a domain.
For example, bob.cozycloud.cc is the instance of Bob. A single cozy-stack
process can manage several instances.

Each instance has a separate space for storing files and a prefix used to
create its CouchDB databases.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var addInstanceCmd = &cobra.Command{
	Use:   "add [domain]",
	Short: "Manage instances of a stack",
	Long: `
cozy-stack instances add allows to create an instance on the cozy for a
given domain.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		domain := args[0]
		if !validDomain(domain) {
			return fmt.Errorf("Invalid domain: %s", domain)
		}

		var dev string
		if flagDev {
			dev = "true"
		} else {
			dev = "false"
		}

		q := url.Values{
			"Domain":   {domain},
			"Apps":     {strings.Join(flagApps, ",")},
			"Locale":   {flagLocale},
			"Timezone": {flagTimezone},
			"Email":    {flagEmail},
			"Dev":      {dev},
		}

		i, err := instancesRequest("POST", "/instances/", q, nil)
		if err != nil {
			log.Errorf("Failed to create instance for domain %s", domain)
			return err
		}

		log.Infof("Instance created with success for domain %s", i.Attrs.Domain)
		log.Infof("Registration token: \"%s\"", hex.EncodeToString(i.Attrs.RegisterToken))
		log.Debugf("Instance created: %#v", i.Attrs)
		return nil
	},
}

var lsInstanceCmd = &cobra.Command{
	Use:   "ls",
	Short: "List instances",
	Long: `
cozy-stack instances ls allows to list all the instances that can be served
by this server.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var doc instancesAPIData
		if err := clientRequestParsed(instancesClient(), "GET", "/instances/", nil, nil, &doc); err != nil {
			return err
		}

		for _, i := range doc.Data {
			var dev string
			if i.Attrs.Dev {
				dev = "dev"
			} else {
				dev = "prod"
			}
			fmt.Printf("%s\t%s\t%s\n", i.Attrs.Domain, i.Attrs.StorageURL, dev)
		}

		return nil
	},
}

var destroyInstanceCmd = &cobra.Command{
	Use:   "destroy [domain]",
	Short: "Remove instance",
	Long: `
cozy-stack instances destroy allows to remove an instance
and all its data.
`,
	Aliases: []string{"rm"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		domain := args[0]
		if !validDomain(domain) {
			return fmt.Errorf("Invalid domain: %s", domain)
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Printf(`
Are you sure you want to remove instance for domain %s ?
All data associated with this domain will be permanently lost.
[yes/NO]: `, domain)

		in, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		if strings.ToLower(strings.TrimSpace(in)) != "yes" {
			return nil
		}

		fmt.Println()

		i, err := instancesRequest("DELETE", "/instances/"+domain, nil, nil)
		if err != nil {
			log.Errorf("Failed to remove instance for domain %s", domain)
			return err
		}

		fmt.Println()

		log.Infof("Instance for domain %s has been destroyed with success", i.Attrs.Domain)
		return nil
	},
}

type instanceData struct {
	ID    string             `json:"id"`
	Rev   string             `json:"rev"`
	Attrs *instance.Instance `json:"attributes"`
}

type instanceAPIData struct {
	Data *instanceData `json:"data"`
}

type instancesAPIData struct {
	Data []*instanceData `json:"data"`
}

func instancesClient() *client {
	var pass []byte

	if !config.IsDevRelease() {
		pass = []byte(os.Getenv("COZY_ADMIN_PASSWORD"))
		if len(pass) == 0 {
			var err error
			fmt.Printf("Password:")
			pass, err = gopass.GetPasswdMasked()
			if err != nil {
				panic(err)
			}
		}
	}

	return &client{
		addr: config.AdminServerAddr(),
		pass: string(pass),
	}
}

func instancesRequest(method, path string, q url.Values, body interface{}) (*instanceData, error) {
	var doc instanceAPIData
	err := clientRequestParsed(instancesClient(), method, path, q, body, &doc)
	if err != nil {
		return nil, err
	}
	return doc.Data, nil
}

func init() {
	instanceCmdGroup.AddCommand(addInstanceCmd)
	instanceCmdGroup.AddCommand(lsInstanceCmd)
	instanceCmdGroup.AddCommand(destroyInstanceCmd)
	addInstanceCmd.Flags().StringVar(&flagLocale, "locale", instance.DefaultLocale, "Locale of the new cozy instance")
	addInstanceCmd.Flags().StringVar(&flagTimezone, "tz", "", "The timezone for the user")
	addInstanceCmd.Flags().StringVar(&flagEmail, "email", "", "The email of the owner")
	addInstanceCmd.Flags().StringSliceVar(&flagApps, "apps", nil, "Apps to be preinstalled")
	addInstanceCmd.Flags().BoolVar(&flagDev, "dev", false, "To create a development instance")
	RootCmd.AddCommand(instanceCmdGroup)
}
