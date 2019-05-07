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

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/keymgmt"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
)

var flagURL string
var flagName string
var flagShasum string
var flagContext string

var configCmdGroup = &cobra.Command{
	Use:   "config <command>",
	Short: "Show and manage configuration elements",
	Long:  `cozy-stack config allows to print and generate some parts of the configuration`,
}

var configPrintCmd = &cobra.Command{
	Use:   "print",
	Short: "Display the configuration",
	Long:  `Read the environment variables, the config file and the given parameters to display the configuration.`,
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
cozy-stack instances passphrase generate a passphrase hash and save it to the specified file. If no file is specified, it is directly printed in standard output. This passphrase is the one used to authenticate accesses to the administration API.

The environment variable 'COZY_ADMIN_PASSPHRASE' can be used to pass the passphrase if needed.
`,
	Example: "$ cozy-stack config passwd ~/.cozy/cozy-admin-passphrase",
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
			f, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0440)
			if err != nil {
				return err
			}
			defer f.Close()

			if err = os.Chmod(filename, 0440); err != nil {
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
cozy-stack config gen-keys generate a key-pair and save them in the specified path.

The decryptor key filename is given the ".dec" extension suffix.
The encryptor key filename is given the ".enc" extension suffix.

The files permissions are 0400.`,

	Example: `$ cozy-stack config gen-keys ~/credentials-key
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

var encryptCredentialsDataCmd = &cobra.Command{
	Use:   "encrypt-data <encoding keyfile> <text>",
	Short: "Encrypt data with the specified encryption keyfile.",
	Long:  `cozy-stack config encrypt-data encrypts any valid JSON data`,
	Example: `
$ ./cozy-stack config encrypt-data ~/.cozy/key.enc "{\"foo\": \"bar\"}"
$ bmFjbNFjY+XZkS26YtVPUIKKm/JdnAGwG30n6A4ypS1p1dHev8hOtaRbW+lGneoO7PS9JCW8U5GSXhASu+c3UkaZ
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Usage()
		}

		// Check if we have good-formatted JSON
		var result map[string]interface{}
		err := json.Unmarshal([]byte(args[1]), &result)
		if err != nil {
			return err
		}

		encKeyStruct, err := readKeyFromFile(args[0])
		if err != nil {
			return err
		}
		dataEncrypted, err := account.EncryptBufferWithKey(encKeyStruct, []byte(args[1]))
		if err != nil {
			return err
		}
		data := base64.StdEncoding.EncodeToString(dataEncrypted)
		fmt.Printf("%s\n", data)

		return nil
	},
}

var decryptCredentialsDataCmd = &cobra.Command{
	Use:   "decrypt-data <decoding keyfile> <ciphertext>",
	Short: "Decrypt data with the specified decryption keyfile.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Usage()
		}

		decKeyStruct, err := readKeyFromFile(args[0])
		if err != nil {
			return err
		}

		dataEncrypted, err := base64.StdEncoding.DecodeString(args[1])
		if err != nil {
			return err
		}
		decrypted, err := account.DecryptBufferWithKey(decKeyStruct, dataEncrypted)
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", decrypted)

		return nil
	},
}

var encryptCredentialsCmd = &cobra.Command{
	Use:     "encrypt-creds <keyfile> <login> <password>",
	Aliases: []string{"encrypt-credentials"},
	Short:   "Encrypt the given credentials with the specified decryption keyfile.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 3 {
			return cmd.Usage()
		}

		credsEncryptor, err := readKeyFromFile(args[0])
		if err != nil {
			return err
		}

		encryptedCreds, err := account.EncryptCredentialsWithKey(credsEncryptor, args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Printf("Encrypted credentials: %s\n", encryptedCreds)
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

		credsDecryptor, err := readKeyFromFile(args[0])
		if err != nil {
			return err
		}

		credentialsEncrypted, err := base64.StdEncoding.DecodeString(args[1])
		if err != nil {
			return fmt.Errorf("Cipher text is not properly base64 encoded: %s", err)
		}

		login, password, err := account.DecryptCredentialsWithKey(credsDecryptor, credentialsEncrypted)
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

func readKeyFromFile(filepath string) (*keymgmt.NACLKey, error) {
	keyBytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	return keymgmt.UnmarshalNACLKey(keyBytes)
}

var insertAssetCmd = &cobra.Command{
	Use:     "insert-asset --url <url> --name <name> --shasum <shasum> --context <context>",
	Short:   "Inserts an asset",
	Long:    "Inserts a custom asset in a specific context",
	Example: "$ cozy-stack config insert-asset --url file:///foo/bar/baz.js --name /foo/bar/baz.js --shasum 0763d6c2cebee0880eb3a9cc25d38cd23db39b5c3802f2dc379e408c877a2788 --context foocontext",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check params
		var customAssets []fs.AssetOption

		if flagContext == "" {
			return fmt.Errorf("You must provide a context")
		}

		assetOption := fs.AssetOption{
			URL:     flagURL,
			Name:    flagName,
			Shasum:  flagShasum,
			Context: flagContext,
		}

		customAssets = append(customAssets, assetOption)

		marshaledAssets, err := json.Marshal(customAssets)
		if err != nil {
			return err
		}

		c := newAdminClient()
		req := &request.Options{
			Method: "POST",
			Path:   "instances/assets",
			Body:   bytes.NewReader(marshaledAssets),
		}
		res, err := c.Req(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		return nil
	},
}

var rmAssetCmd = &cobra.Command{
	Use:     "rm-asset [context] [name]",
	Short:   "Removes an asset",
	Long:    "Removes a custom asset in a specific context",
	Example: "$ cozy-stack config rm-asset foobar /foo/bar/baz.js",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check params
		if len(args) != 2 {
			return cmd.Usage()
		}

		c := newAdminClient()
		req := &request.Options{
			Method: "DELETE",
			Path:   fmt.Sprintf("instances/assets/%s/%s", args[0], args[1]),
		}
		res, err := c.Req(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		return nil
	},
}

var listAssetCmd = &cobra.Command{
	Use:     "ls-assets",
	Short:   "List assets",
	Long:    "List assets currently served by the stack",
	Example: "$ cozy-stack config ls-assets",
	RunE: func(cmd *cobra.Command, args []string) error {

		c := newAdminClient()
		req := &request.Options{
			Method: "GET",
			Path:   "instances/assets",
		}
		res, err := c.Req(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		var v interface{}

		err = json.NewDecoder(res.Body).Decode(&v)
		if err != nil {
			return err
		}

		json, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(json))
		return nil
	},
}

var listContextsCmd = &cobra.Command{
	Use:     "ls-contexts",
	Short:   "List contexts",
	Long:    "List contexts currently used by the stack",
	Example: "$ cozy-stack config ls-contexts",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newAdminClient()
		req := &request.Options{
			Method: "GET",
			Path:   "instances/contexts",
		}
		res, err := c.Req(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		var v interface{}

		err = json.NewDecoder(res.Body).Decode(&v)
		if err != nil {
			return err
		}

		json, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(json))
		return nil
	},
}

func init() {
	configCmdGroup.AddCommand(configPrintCmd)
	configCmdGroup.AddCommand(adminPasswdCmd)
	configCmdGroup.AddCommand(genKeysCmd)
	configCmdGroup.AddCommand(encryptCredentialsDataCmd)
	configCmdGroup.AddCommand(decryptCredentialsDataCmd)
	configCmdGroup.AddCommand(encryptCredentialsCmd)
	configCmdGroup.AddCommand(decryptCredentialsCmd)
	configCmdGroup.AddCommand(insertAssetCmd)
	configCmdGroup.AddCommand(listAssetCmd)
	configCmdGroup.AddCommand(rmAssetCmd)
	configCmdGroup.AddCommand(listContextsCmd)
	RootCmd.AddCommand(configCmdGroup)
	insertAssetCmd.Flags().StringVar(&flagURL, "url", "", "The URL of the asset")
	insertAssetCmd.Flags().StringVar(&flagName, "name", "", "The name of the asset")
	insertAssetCmd.Flags().StringVar(&flagShasum, "shasum", "", "The shasum of the asset")
	insertAssetCmd.Flags().StringVar(&flagContext, "context", "", "The context of the asset")
}
