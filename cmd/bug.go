package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/cozy/cozy-stack/cmd/browser"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/spf13/cobra"
)

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
		b.Append("cozy-stack %s", config.Version)
		b.Append("build in mode %s - %s\n", config.BuildMode, config.BuildTime)
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
	RootCmd.AddCommand(bugCmd)
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
	cmd := exec.Command(path, args...) // #nosec
	out, err := cmd.Output()
	if err != nil {
		return
	}
	fmt.Fprintf(w, "%s%s\n", prefix, bytes.TrimSpace(out))
}
