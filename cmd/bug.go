package cmd

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
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
		var buf bytes.Buffer
		buf.WriteString(bugHeader)
		fmt.Fprint(&buf, "#### System details\n\n")
		fmt.Fprintln(&buf, "```")
		fmt.Fprintf(&buf, "cozy-stack %s\n", config.Version)
		fmt.Fprintf(&buf, "build in mode %s - %s\n\n", config.BuildMode, config.BuildTime)
		fmt.Fprintf(&buf, "go version %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
		printOSDetails(&buf)
		fmt.Fprintln(&buf, "```")
		body := buf.String()
		url := "https://github.com/cozy/cozy-stack/issues/new?body=" + url.QueryEscape(body)
		if !browser.Open(url) {
			fmt.Print("Please file a new issue at golang.org/issue/new using this template:\n\n")
			fmt.Print(body)
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

	case "solaris":
		out, err := ioutil.ReadFile("/etc/release")
		if err == nil {
			fmt.Fprintf(w, "/etc/release: %s\n", out)
		}
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
