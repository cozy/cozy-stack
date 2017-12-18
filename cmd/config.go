package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
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
	Use:     "passwd [filepath]",
	Aliases: []string{"password", "passphrase", "pass"},
	Short:   "Generate an admin passphrase",
	Long: `
cozy-stack instances passphrase generate a passphrase hash and save it to the
specified file. If no file is specified, it is directly printed in standard output.
This passphrase is the one used to authenticate accesses to the administration API.

The environment variable 'COZY_ADMIN_PASSPHRASE' can be used to pass the passphrase
if needed.

example: cozy-stack config passwd ~/.cozy/
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return cmd.Usage()
		}
		var filename string
		if len(args) == 1 {
			filename = filepath.Join(utils.AbsPath(args[0]))
			ok, err := utils.DirExists(filename)
			if err == nil && ok {
				filename = path.Join(filename, config.GetConfig().AdminSecretFileName)
			}
		}

		if filename != "" {
			errPrintfln("Hashed passphrase will be written in %s", filename)
		}

		passphrase := []byte(os.Getenv("COZY_ADMIN_PASSPHRASE"))
		if len(passphrase) == 0 {
			errPrintf("Passphrase: ")
			pass1, err := gopass.GetPasswdPrompt("", false, os.Stdin, os.Stderr)
			if err != nil {
				return err
			}

			errPrintf("Confirmation: ")
			pass2, err := gopass.GetPasswdPrompt("", false, os.Stdin, os.Stderr)
			if err != nil {
				return err
			}
			if !bytes.Equal(pass1, pass2) {
				return fmt.Errorf("Passphrase missmatch")
			}

			passphrase = pass1
		}

		b, err := crypto.GenerateFromPassphrase(passphrase)
		if err != nil {
			return err
		}

		var out io.Writer
		if filename != "" {
			var f *os.File
			f, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0444)
			if err != nil {
				return err
			}
			defer f.Close()

			if err = os.Chmod(filename, 0444); err != nil {
				return err
			}

			out = f
		} else {
			out = os.Stdout
		}

		_, err = fmt.Fprintln(out, string(b))
		return err
	},
}

func init() {
	configCmdGroup.AddCommand(configPrintCmd)
	configCmdGroup.AddCommand(adminPasswdCmd)
	RootCmd.AddCommand(configCmdGroup)
}
