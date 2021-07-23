package cmd

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/cozy/cozy-stack/cmd/browser"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/spf13/cobra"
)

var toolsCmdGroup = &cobra.Command{
	Use:   "tools <command>",
	Short: "Regroup some tools for debugging and tests",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var encryptRSACmd = &cobra.Command{
	Use:   "encrypt-with-rsa <key> <payload",
	Short: "encrypt a payload in RSA",
	Long: `
This command is used by integration tests to encrypt bitwarden organization
keys. It takes the symmetric key and the payload (= the organization key) as
inputs (both encoded in base64), and print on stdout the encrypted data
(encoded as base64 too).
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Usage()
		}
		privateKey, err := base64.StdEncoding.DecodeString(args[0])
		if err != nil {
			return err
		}
		payload, err := base64.StdEncoding.DecodeString(args[1])
		if err != nil {
			return err
		}
		key, err := x509.ParsePKCS8PrivateKey(privateKey)
		if err != nil {
			return err
		}
		pub := key.(*rsa.PrivateKey).PublicKey
		hash := sha1.New()
		rng := rand.Reader
		encrypted, err := rsa.EncryptOAEP(hash, rng, &pub, payload, nil)
		if err != nil {
			return err
		}
		fmt.Printf("4.%s", base64.StdEncoding.EncodeToString(encrypted))
		return nil
	},
}

const bugHeader = `Please answer these questions before submitting your issue. Thanks!


#### What did you do?

If possible, provide a recipe for reproducing the error.


#### What did you expect to see?


#### What did you see instead?


`

type body struct {
	buf bytes.Buffer
	err error
}

func (b *body) Append(format string, args ...interface{}) {
	if b.err != nil {
		return
	}
	_, b.err = fmt.Fprintf(&b.buf, format+"\n", args...)
}

func (b *body) String() string {
	return b.buf.String()
}

// bugCmd represents the `cozy-stack bug` command, inspired from go bug.
// Cf https://tip.golang.org/src/cmd/go/internal/bug/bug.go
var bugCmd = &cobra.Command{
	Use:   "bug",
	Short: "start a bug report",
	Long: `
Bug opens the default browser and starts a new bug report.
The report includes useful system information.
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var b body
		b.Append("%s", bugHeader)
		b.Append("#### System details\n")
		b.Append("```")
		b.Append("cozy-stack %s", build.Version)
		b.Append("build in mode %s - %s\n", build.BuildMode, build.BuildTime)
		b.Append("go version %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
		printOSDetails(&b.buf)
		b.Append("```")
		if b.err != nil {
			return b.err
		}
		param := url.QueryEscape(b.String())
		url := "https://github.com/cozy/cozy-stack/issues/new?body=" + param
		if !browser.Open(url) {
			fmt.Print("Please file a new issue at https://github.com/cozy/cozy-stack/issues/new using this template:\n\n")
			fmt.Print(b.String())
		}
		return nil
	},
}

func init() {
	toolsCmdGroup.AddCommand(encryptRSACmd)
	toolsCmdGroup.AddCommand(bugCmd)
	RootCmd.AddCommand(toolsCmdGroup)
}

func printOSDetails(w io.Writer) {
	switch runtime.GOOS {
	case "darwin":
		printCmdOut(w, "uname -v: ", "uname", "-v")
		printCmdOut(w, "", "sw_vers")

	case "linux":
		printCmdOut(w, "uname -sr: ", "uname", "-sr")
		printCmdOut(w, "", "lsb_release", "-a")

	case "openbsd", "netbsd", "freebsd", "dragonfly":
		printCmdOut(w, "uname -v: ", "uname", "-v")
	}
}

// printCmdOut prints the output of running the given command.
// It ignores failures; 'go bug' is best effort.
func printCmdOut(w io.Writer, prefix, path string, args ...string) {
	cmd := exec.Command(path, args...)
	out, err := cmd.Output()
	if err != nil {
		return
	}
	fmt.Fprintf(w, "%s%s\n", prefix, bytes.TrimSpace(out))
}
