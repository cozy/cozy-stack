package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/spf13/cobra"
)

var flagSwiftObjectContentType string
var flagShowDomains bool

var swiftCmdGroup = &cobra.Command{
	Use:   "swift <command>",
	Short: "Interact directly with OpenStack Swift object storage",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var lsLayoutsCmd = &cobra.Command{
	Use:     "ls-layouts",
	Short:   `Count layouts by types (v1, v2a, v2b, v3a, v3b)`,
	Example: "$ cozy-stack swift ls-layouts",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newAdminClient()
		values := url.Values{}
		values.Add("show_domains", strconv.FormatBool(flagShowDomains))
		res, err := c.Req(&request.Options{
			Method:  "GET",
			Path:    "/swift/layouts",
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
	Use:     "get <domain> <object-name>",
	Aliases: []string{"download"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Usage()
		}

		c := newAdminClient()
		path := fmt.Sprintf("/swift/vfs/%s", url.PathEscape(args[1]))
		res, err := c.Req(&request.Options{
			Method: "GET",
			Path:   path,
			Domain: args[0],
		})
		if err != nil {
			return err
		}
		defer res.Body.Close()

		// Read the body and print it
		_, err = io.Copy(os.Stdout, res.Body)
		return err
	},
}

var swiftPutCmd = &cobra.Command{
	Use:     "put <domain> <object-name>",
	Aliases: []string{"upload"},
	Long: `cozy-stack swift put can be used to create or update an object in
the swift container associated to the given domain. The content of the file is
expected on the standard input.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Usage()
		}

		c := newAdminClient()
		buf := new(bytes.Buffer)

		_, err := io.Copy(buf, os.Stdin)
		if err != nil {
			return err
		}

		_, err = c.Req(&request.Options{
			Method: "PUT",
			Path:   fmt.Sprintf("/swift/vfs/%s", url.PathEscape(args[1])),
			Body:   bytes.NewReader(buf.Bytes()),
			Domain: args[0],
			Headers: map[string]string{
				"Content-Type": flagSwiftObjectContentType,
			},
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

		c := newAdminClient()
		path := fmt.Sprintf("/swift/vfs/%s", url.PathEscape(args[1]))
		_, err := c.Req(&request.Options{
			Method: "DELETE",
			Path:   path,
			Domain: args[0],
		})

		return err
	},
}

var swiftLsCmd = &cobra.Command{
	Use:     "ls <domain>",
	Aliases: []string{"list"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return cmd.Usage()
		}

		type resStruct struct {
			ObjectNameList []string `json:"objects_names"`
		}

		c := newAdminClient()
		res, err := c.Req(&request.Options{
			Method: "GET",
			Path:   "/swift/vfs",
			Domain: args[0],
		})
		if err != nil {
			return err
		}

		names := resStruct{}
		err = json.NewDecoder(res.Body).Decode(&names)
		if err != nil {
			return err
		}

		for _, name := range names.ObjectNameList {
			fmt.Println(name)
		}

		return nil
	},
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
