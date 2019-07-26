package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/ncw/swift"
	"github.com/spf13/cobra"
)

var flagSwiftObjectContentType string
var flagShowDomains bool

var swiftCmdGroup = &cobra.Command{
	Use:   "swift <command>",
	Short: "Interact directly with OpenStack Swift object storage",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Setup(cfgFile); err != nil {
			return err
		}
		if config.FsURL().Scheme != config.SchemeSwift &&
			config.FsURL().Scheme != config.SchemeSwiftSecure {
			return fmt.Errorf("swift: the configured filesystem does not rely on OpenStack Swift")
		}
		return config.InitSwiftConnection(config.GetConfig().Fs)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var lsLayoutsCmd = &cobra.Command{
	Use:     "ls-layouts",
	Short:   `Count layouts by types (v1, v2a, v2b, v3)`,
	Example: "$ cozy-stack swift ls-layouts",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newAdminClient()
		values := url.Values{}
		values.Add("show_domains", strconv.FormatBool(flagShowDomains))
		res, err := c.Req(&request.Options{
			Method:  "GET",
			Path:    "/swift/list-layouts",
			Queries: values,
		})
		if err != nil {
			return err
		}
		defer res.Body.Close()

		var buf interface{}
		if err := json.NewDecoder(res.Body).Decode(&buf); err != nil {
			return err
		}
		json, err := json.MarshalIndent(buf, "", "  ")

		if err != nil {
			return err
		}

		fmt.Println(string(json))
		return nil
	},
}

var swiftGetCmd = &cobra.Command{
	Use: "get <domain> <object-name>",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Usage()
		}
		type reqStruct struct {
			Instance   string `json:"instance"`
			ObjectName string `json:"object_name"`
		}

		reqBody := reqStruct{
			Instance:   args[0],
			ObjectName: args[1],
		}

		body, err := json.Marshal(&reqBody)
		if err != nil {
			return err
		}

		c := newAdminClient()
		res, err := c.Req(&request.Options{
			Method: "POST",
			Path:   "/swift/get",
			Body:   bytes.NewReader(body),
		})
		if err != nil {
			return err
		}
		defer res.Body.Close()

		// Get the object
		type resStruct struct {
			Content string `json:"content"`
		}
		var out resStruct
		err = json.NewDecoder(res.Body).Decode(&out)
		if err != nil {
			return err
		}

		fmt.Println(out.Content)

		return err

	},
}

var swiftPutCmd = &cobra.Command{
	Use: "put <domain> <object-name>",
	Long: `cozy-stack swift put can be used to create or update an object in
the swift container associated to the given domain. The content of the file is
expected on the standard input.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Usage()
		}

		type reqStruct struct {
			Instance    string `json:"instance"`
			ObjectName  string `json:"object_name"`
			Content     string `json:"content"`
			ContentType string `json:"content_type"`
		}

		c := newAdminClient()
		var buf = new(bytes.Buffer)

		_, err := io.Copy(buf, os.Stdin)
		if err != nil {
			return err
		}

		body, err := json.Marshal(reqStruct{
			Instance:    args[0],
			ObjectName:  args[1],
			Content:     buf.String(),
			ContentType: flagSwiftObjectContentType,
		})
		if err != nil {
			return err
		}

		_, err = c.Req(&request.Options{
			Method: "POST",
			Path:   "/swift/put",
			Body:   bytes.NewReader(body),
		})
		if err != nil {
			return err
		}

		fmt.Println("Object has been added to swift")
		return nil
	},
}

var swiftDeleteCmd = &cobra.Command{
	Use:     "rm <domain> <object-name>",
	Aliases: []string{"delete"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Usage()
		}
		i, err := lifecycle.GetInstance(args[0])
		if err != nil {
			return err
		}
		sc := config.GetSwiftConnection()
		objectName := args[1]
		return sc.ObjectDelete(swiftContainer(i), objectName)
	},
}

var swiftLsCmd = &cobra.Command{
	Use: "ls <domain>",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return cmd.Usage()
		}
		i, err := lifecycle.GetInstance(args[0])
		if err != nil {
			return err
		}
		sc := config.GetSwiftConnection()
		container := swiftContainer(i)
		return sc.ObjectsWalk(container, nil, func(opts *swift.ObjectsOpts) (interface{}, error) {
			names, err := sc.ObjectNames(container, opts)
			if err == nil {
				fmt.Println(strings.Join(names, "\n"))
			}
			return names, err
		})
	},
}

func swiftContainer(i *instance.Instance) string {
	switch i.SwiftLayout {
	case 0:
		return "cozy-" + i.DBPrefix()
	case 1:
		return "cozy-v2-" + i.DBPrefix()
	case 2:
		return "cozy-v3-" + i.DBPrefix()
	default:
		panic(errors.New("Unknown Swift layout"))
	}
}

func init() {
	swiftPutCmd.Flags().StringVar(&flagSwiftObjectContentType, "content-type", "", "Specify a Content-Type for the created object")
	lsLayoutsCmd.Flags().BoolVar(&flagShowDomains, "show-domains", false, "Show the domains along the counter")

	swiftCmdGroup.AddCommand(swiftGetCmd)
	swiftCmdGroup.AddCommand(swiftPutCmd)
	swiftCmdGroup.AddCommand(swiftDeleteCmd)
	swiftCmdGroup.AddCommand(swiftLsCmd)
	swiftCmdGroup.AddCommand(lsLayoutsCmd)

	RootCmd.AddCommand(swiftCmdGroup)
}
