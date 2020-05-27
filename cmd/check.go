package cmd

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/spf13/cobra"
)

var flagCheckFSIndexIntegrity bool
var flagCheckFSFilesConsistensy bool
var flagCheckFSFailFast bool

var checkCmdGroup = &cobra.Command{
	Use:   "check <command>",
	Short: "A set of tools to check that instances are in the expected state.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var checkFSCmd = &cobra.Command{
	Use:   "fs <domain>",
	Short: "Check a vfs",
	Long: `
This command checks that the files in the VFS are not desynchronized, ie a file
present in CouchDB but not swift/localfs, or present in swift/localfs but not
couchdb.

There are 2 steps:

- index integrity checks that there are nothing wrong in the index (CouchDB),
  like a file present in a directory that has been deleted
- files consistency checks that the files are the same in the index (CouchDB)
  and the storage (Swift or localfs).

By default, both operations are done, but you can choose one or the other via
the flags.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
		}
		return fsck(args[0])
	},
}

func fsck(domain string) error {
	if flagCheckFSFilesConsistensy && flagCheckFSIndexIntegrity {
		flagCheckFSIndexIntegrity = false
		flagCheckFSFilesConsistensy = false
	}

	c := newAdminClient()
	res, err := c.Req(&request.Options{
		Method: "GET",
		Path:   "/instances/" + url.PathEscape(domain) + "/fsck",
		Queries: url.Values{
			"IndexIntegrity":   {strconv.FormatBool(flagCheckFSIndexIntegrity)},
			"FilesConsistency": {strconv.FormatBool(flagCheckFSFilesConsistensy)},
			"FailFast":         {strconv.FormatBool(flagCheckFSFailFast)},
		},
	})
	if err != nil {
		return err
	}

	hasLogs := false
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		hasLogs = true
		fmt.Println(string(scanner.Bytes()))
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if hasLogs {
		os.Exit(1)
	}
	return nil
}

func init() {
	checkCmdGroup.AddCommand(checkFSCmd)
	checkFSCmd.Flags().BoolVar(&flagCheckFSIndexIntegrity, "index-integrity", false, "Check the index integrity only")
	checkFSCmd.Flags().BoolVar(&flagCheckFSFilesConsistensy, "files-consistency", false, "Check the files consistency only (between CouchDB and Swift)")
	checkFSCmd.Flags().BoolVar(&flagCheckFSFailFast, "fail-fast", false, "Stop the FSCK on the first error")

	RootCmd.AddCommand(checkCmdGroup)
}
