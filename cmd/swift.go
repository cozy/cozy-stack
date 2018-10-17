package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/swift"
	"github.com/spf13/cobra"
)

var flagSwiftObjectContentType string

var swiftCmdGroup = &cobra.Command{
	Use:   "swift <command>",
	Short: "Interact directly with OpenStack Swift object storage",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Setup(cfgFile); err != nil {
			return err
		}
		if config.FsURL().Scheme != config.SchemeSwift {
			return fmt.Errorf("swift: the configured filesystem does not rely on OpenStack Swift")
		}
		return config.InitSwiftConnection(config.FsURL())
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var swiftGetCmd = &cobra.Command{
	Use: "get <domain> <object-name>",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Usage()
		}
		i, err := instance.Get(args[0])
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
		i, err := instance.Get(args[0])
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
		i, err := instance.Get(args[0])
		if err != nil {
			return err
		}
		sc := config.GetSwiftConnection()
		objectName := args[1]
		return sc.ObjectDelete(swiftContainer(i), objectName)
	},
}

var swiftLsCmd = &cobra.Command{
	Use:     "ls <domain>",
	Aliases: []string{"delete"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return cmd.Usage()
		}
		i, err := instance.Get(args[0])
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

	swiftCmdGroup.AddCommand(swiftGetCmd)
	swiftCmdGroup.AddCommand(swiftPutCmd)
	swiftCmdGroup.AddCommand(swiftDeleteCmd)
	swiftCmdGroup.AddCommand(swiftLsCmd)

	RootCmd.AddCommand(swiftCmdGroup)
}
