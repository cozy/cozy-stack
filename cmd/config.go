package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
)

var configCmdGroup = &cobra.Command{
	Use:   "config [command]",
	Short: "Show and manage configuration elements",
	Long: `
cozy-stack config allows to print and generate some parts of the configuration
`,
}

var configPrintCmd = &cobra.Command{
	Use:   "print",
	Short: "Display the configuration",
	Long: `Read the environment variables, the config file and
the given parameters to display the configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := json.MarshalIndent(config.GetConfig(), "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(cfg))
		return nil
	},
}

var adminPasswdCmd = &cobra.Command{
	Use:   "passwd [directory]",
	Short: "Generate an admin passphrase",
	Long: `
cozy-stack instances passphrase generate a passphrase hash and save it to a file in
the specified directory. This passphrase is the one used to authenticate accesses
to the administration API.

example: cozy-stack config passwd ~/.cozy
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		directory := filepath.Join(utils.AbsPath(args[0]))
		err := os.MkdirAll(directory, 0700)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(os.Stdout, "Passphrase:")
		if err != nil {
			return err
		}
		pass1, err := gopass.GetPasswdMasked()
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(os.Stdout, "Confirmation:")
		if err != nil {
			return err
		}
		pass2, err := gopass.GetPasswdMasked()
		if err != nil {
			return err
		}
		if !bytes.Equal(pass1, pass2) {
			return fmt.Errorf("Passphrase missmatch")
		}

		b, err := crypto.GenerateFromPassphrase(pass1)
		if err != nil {
			return err
		}

		filename := filepath.Join(directory, config.AdminSecretFileName)
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0444)
		if err != nil {
			return err
		}
		defer f.Close()

		if err = os.Chmod(filename, 0444); err != nil {
			return err
		}

		_, err = fmt.Fprintln(f, string(b))
		if err != nil {
			return err
		}

		fmt.Println("Hashed passphrase outputted in", filename)
		return nil
	},
}

func init() {
	configCmdGroup.AddCommand(configPrintCmd)
	configCmdGroup.AddCommand(adminPasswdCmd)
	RootCmd.AddCommand(configCmdGroup)
}
