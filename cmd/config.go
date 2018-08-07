package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/cozy/cozy-stack/pkg/accounts"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/keymgmt"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
)

var configCmdGroup = &cobra.Command{
	Use:   "config <command>",
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
	Use:     "passwd <filepath>",
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

var genKeysCmd = &cobra.Command{
	Use:   "gen-keys <filepath>",
	Short: "Generate an key pair for encryption and decryption of credentials",
	Long: `
cozy-stack config gen-keys generate a key-pair and save them in the
specified path.

The decryptor key filename is given the ".dec" extension suffix.
The encryptor key filename is given the ".enc" extension suffix.

The files permissions are 0400.

example: cozy-stack config gen-keys ~/credentials-key
keyfiles written in:
	~/credentials-key.enc
	~/credentials-key.dec
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}

		filename := filepath.Join(utils.AbsPath(args[0]))
		encryptorFilename := filename + ".enc"
		decryptorFilename := filename + ".dec"

		marshaledEncryptorKey, marshaledDecryptorKey, err := keymgmt.GenerateEncodedNACLKeyPair()
		if err != nil {
			return nil
		}
		if err = writeFile(encryptorFilename, marshaledEncryptorKey, 0400); err != nil {
			return err
		}
		if err = writeFile(decryptorFilename, marshaledDecryptorKey, 0400); err != nil {
			return err
		}
		errPrintfln("keyfiles written in:\n  %s\n  %s", encryptorFilename, decryptorFilename)
		return nil
	},
}

var decryptCredentialsCmd = &cobra.Command{
	Use:     "decrypt-creds <keyfile> <ciphertext>",
	Aliases: []string{"decrypt-credentials"},
	Short:   "Decrypt the given credentials cipher text with the specified decryption keyfile.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Usage()
		}

		keyBytes, err := ioutil.ReadFile(args[0])
		if err != nil {
			return err
		}
		credsDecryptor, err := keymgmt.UnmarshalNACLKey(keyBytes)
		if err != nil {
			return err
		}

		credentialsEncrypted, err := base64.StdEncoding.DecodeString(args[1])
		if err != nil {
			return fmt.Errorf("Cipher text is not properly base64 encoded: %s", err)
		}

		login, password, err := accounts.DecryptCredentialsWithKey(credsDecryptor, credentialsEncrypted)
		if err != nil {
			return fmt.Errorf("Could not decrypt cipher text: %s", err)
		}

		fmt.Printf(`Decrypted credentials:
login:    %q
password: %q
`, login, password)
		return nil
	},
}

func writeFile(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}
	return err
}

func init() {
	configCmdGroup.AddCommand(configPrintCmd)
	configCmdGroup.AddCommand(adminPasswdCmd)
	configCmdGroup.AddCommand(genKeysCmd)
	configCmdGroup.AddCommand(decryptCredentialsCmd)
	RootCmd.AddCommand(configCmdGroup)
}
