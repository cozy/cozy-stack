package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/swift"
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
	Short:   `Count layouts by types (v1, v2a, v2b)`,
	Example: "$ cozy-stack swift ls-layouts",
	RunE: func(cmd *cobra.Command, args []string) error {
		type layout struct {
			Counter int      `json:"counter"`
			Domains []string `json:"domains,omitempty"`
		}
		var layoutV1, layoutV2a, layoutV2b, layoutUnknown layout

		instances, err := instance.List()
		if err != nil {
			return err
		}
		for _, inst := range instances {
			if inst.SwiftCluster == 0 {
				layoutV1.Counter++
				if flagShowDomains {
					layoutV1.Domains = append(layoutV1.Domains, inst.Domain)
				}
			} else { // v2
				switch inst.DBPrefix() {
				case inst.Domain:
					layoutV2a.Counter++
					if flagShowDomains {
						layoutV2a.Domains = append(layoutV2a.Domains, inst.Domain)
					}
				case inst.Prefix:
					layoutV2b.Counter++
					if flagShowDomains {
						layoutV2b.Domains = append(layoutV2b.Domains, inst.Domain)
					}
				default:
					layoutUnknown.Counter++
					if flagShowDomains {
						layoutUnknown.Domains = append(layoutUnknown.Domains, inst.Domain)
					}
				}
			}
		}

		output := make(map[string]interface{})
		output["v1"] = layoutV1
		output["v2a"] = layoutV2a
		output["v2b"] = layoutV2b
		output["unknown"] = layoutUnknown
		output["total"] = layoutV1.Counter + layoutV2a.Counter + layoutV2b.Counter + layoutUnknown.Counter

		json, err := json.MarshalIndent(output, "", "  ")

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
		i, err := lifecycle.GetInstance(args[0])
		if err != nil {
			return err
		}
		sc := config.GetSwiftConnection()
		objectName := args[1]
		f, _, err := sc.ObjectOpen(swiftContainer(i), objectName, false, nil)
		if err != nil {
			return err
		}
		_, err = io.Copy(os.Stdout, f)
		if err != nil {
			return err
		}
		return f.Close()
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
		i, err := lifecycle.GetInstance(args[0])
		if err != nil {
			return err
		}
		sc := config.GetSwiftConnection()
		objectName := args[1]
		f, err := sc.ObjectCreate(swiftContainer(i), objectName, true, "", flagSwiftObjectContentType, nil)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, os.Stdin)
		if err != nil {
			return nil
		}
		return f.Close()
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
	if i.SwiftCluster > 0 {
		return "cozy-v2-" + i.DBPrefix()
	}
	return "cozy-" + i.DBPrefix()
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
