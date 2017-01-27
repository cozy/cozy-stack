package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var errFilesExec = errors.New("Bad usage of files exec")

const filesExecUsage = `Available commands:

  mkdir [name]               Creates a directory with specified name
  ls [-l] [-a] [-h] [name]   Prints the children of the specified directory
  tree [name]                Prints the tree structure of the specified directory
  attrs [name]               Prints the attributes of the specified file or directory
  cat [name]                 Echo the file content in stdout
  mv [from] [to]             Rename a file or directory
  rm [-f] [-r] [name]        Move the file to trash, or delete it permanently with -f flag
  restore [name]             Restore a file or directory from trash
`

var flagImportFrom string
var flagImportTo string
var flagImportDryRun bool
var flagImportMatch string

// filesCmdGroup represents the instances command
var filesCmdGroup = &cobra.Command{
	Use:   "files [command]",
	Short: "Interact with the cozy filesystem",
	Long: `
cozy-stack files allows to interact with the cozy filesystem.

It provides command to create, move copy or delete files and
directories inside your cozy instance, using the command line
interface. It also provide an import command to import from your
current filesystem into cozy.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var execFilesCmd = &cobra.Command{
	Use:   "exec [domain] [command]",
	Short: "Execute the given command on the specified domain and leave",
	Long:  "Execute a command on the VFS of the specified domain.\n" + filesExecUsage,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Help()
		}

		domain, command := args[0], args[1]
		c, err := getInstance(domain)
		if err != nil {
			return err
		}

		err = execCommand(c, command, os.Stdout)
		if err == errFilesExec {
			return err
		}

		return err
	},
}

var importFilesCmd = &cobra.Command{
	Use:   "import [domain] [--from name] [--to name] [--match pattern]",
	Short: "Import the specified file or directory into cozy",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		if flagImportFrom == "" || flagImportTo == "" {
			return cmd.Help()
		}

		domain := args[0]
		c, err := getInstance(domain)
		if err != nil {
			return err
		}

		var match *regexp.Regexp
		if flagImportMatch != "" {
			match, err = regexp.Compile(flagImportMatch)
			if err != nil {
				return err
			}
		}

		return importFiles(c, flagImportFrom, flagImportTo, match)
	},
}

func execCommand(c *instance.Instance, command string, w io.Writer) error {
	args := splitArgs(command)
	if len(args) == 0 {
		return errFilesExec
	}

	cmdname := args[0]

	fset := flag.NewFlagSet("", flag.ContinueOnError)

	var flagMkdirP bool
	var flagLsVerbose bool
	var flagLsHuman bool
	var flagLsAll bool
	var flagRmForce bool
	var flagRmRecur bool

	switch cmdname {
	case "mkdir":
		fset.BoolVar(&flagMkdirP, "p", false, "Create imtermediary directories")
	case "ls":
		fset.BoolVar(&flagLsVerbose, "l", false, "List in with additional attributes")
		fset.BoolVar(&flagLsHuman, "h", false, "Print size in human readable format")
		fset.BoolVar(&flagLsAll, "a", false, "Print hidden directories")
	case "rm":
		fset.BoolVar(&flagRmForce, "f", false, "Delete file or directory permanently")
		fset.BoolVar(&flagRmRecur, "r", false, "Delete directory and all its contents")
	}

	if err := fset.Parse(args[1:]); err != nil {
		return err
	}

	args = fset.Args()
	if len(args) == 0 {
		return errFilesExec
	}

	switch cmdname {
	case "mkdir":
		return mkdirCmd(c, args[0], flagMkdirP)
	case "ls":
		return lsCmd(c, args[0], w, flagLsVerbose, flagLsHuman, flagLsAll)
	case "tree":
		return treeCmd(c, args[0], w)
	case "attrs":
		return attrsCmd(c, args[0], w)
	case "cat":
		return catCmd(c, args[0], w)
	case "mv":
		if len(args) < 2 {
			return errFilesExec
		}
		return mvCmd(c, args[0], args[1])
	case "rm":
		return rmCmd(c, args[0], flagRmForce, flagRmRecur)
	case "restore":
		return restoreCmd(c, args[0])
	}

	return errFilesExec
}

func mkdirCmd(c *instance.Instance, name string, mkdirP bool) error {
	q := url.Values{}
	q.Add("Path", name)
	q.Add("Type", "directory")
	if mkdirP {
		q.Add("Recursive", "true")
	}
	return clientRequestAndClose(filesClient(c), "POST", "/files/", q, nil)
}

func lsCmd(c *instance.Instance, root string, w io.Writer, verbose, human, all bool) error {
	q := url.Values{}
	q.Add("Path", root)
	doc, err := filesRequest(c, "GET", "/files/metadata", q, nil)
	if err != nil {
		return err
	}

	type filePrint struct {
		typ   string
		name  string
		size  string
		mdate string
		exec  string
	}

	now := time.Now()

	var prints []*filePrint
	var maxnamelen int
	var maxsizelen int

	for _, f := range doc.Included {
		attrs := f.Attrs
		var typ, name, size, mdate, exec string

		name = attrs.Name

		if now.Year() == attrs.UpdatedAt.Year() {
			mdate = attrs.UpdatedAt.Format("Jan 02 15:04")
		} else {
			mdate = attrs.UpdatedAt.Format("Jan 02 2015")
		}

		if f.Attrs.Type == consts.DirType {
			typ = "d"
			exec = "x"
		} else {
			typ = ""
			if attrs.Executable {
				exec = "x"
			} else {
				exec = "-"
			}
			if human {
				size = humanize.Bytes(uint64(attrs.Size))
			} else {
				size = humanize.Comma(attrs.Size)
			}
		}

		if len(name) > maxnamelen {
			maxnamelen = len(name)
		}

		if len(size) > maxsizelen {
			maxsizelen = len(size)
		}

		if all || len(name) == 0 || name[0] != '.' {
			prints = append(prints, &filePrint{
				typ:   typ,
				name:  name,
				size:  size,
				mdate: mdate,
				exec:  exec,
			})
		}
	}

	if !verbose {
		for _, fp := range prints {
			_, err = fmt.Fprintln(w, fp.name)
			if err != nil {
				return err
			}
		}
		return nil
	}

	smaxsizelen := strconv.Itoa(maxsizelen)
	smaxnamelen := strconv.Itoa(maxnamelen)

	for _, fp := range prints {
		_, err = fmt.Fprintf(w, "%1s%s  %"+smaxsizelen+"s %s %-"+smaxnamelen+"s\n",
			fp.typ, fp.exec, fp.size, fp.mdate, fp.name)
		if err != nil {
			return err
		}
	}

	return nil
}

func treeCmd(c *instance.Instance, root string, w io.Writer) error {
	q := url.Values{}
	q.Add("Path", root)

	doc, err := filesRequest(c, "GET", "/files/metadata", q, nil)
	if err != nil {
		return err
	}

	if doc.Data.ID == consts.RootDirID {
		_, err = fmt.Fprintln(w, "/")
	} else {
		_, err = fmt.Fprintln(w, doc.Data.Attrs.Name)
	}

	if err != nil {
		return err
	}

	return treeRecurs(c, doc, root, 2, w)
}

func treeRecurs(c *instance.Instance, doc *fileAPIData, root string, level int, w io.Writer) error {
	for _, f := range doc.Included {
		for i := 0; i < level-1; i++ {
			var err error
			if i == level-2 {
				_, err = fmt.Fprintf(w, "└── ")
			} else {
				_, err = fmt.Fprintf(w, "|  ")
			}
			if err != nil {
				return err
			}
		}

		name := f.Attrs.Name
		_, err := fmt.Fprintln(w, name)
		if err != nil {
			return err
		}

		if f.Attrs.Type == consts.DirType {
			var child *fileAPIData
			child, err = filesRequest(c, "GET", "/files/"+f.ID, nil, nil)
			if err != nil {
				return err
			}
			err = treeRecurs(c, child, path.Join(root, name), level+1, w)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func attrsCmd(c *instance.Instance, name string, w io.Writer) error {
	q := url.Values{}
	q.Add("Path", name)

	doc, err := filesRequest(c, "GET", "/files/metadata", q, nil)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(doc)
}

func catCmd(c *instance.Instance, name string, w io.Writer) error {
	q := url.Values{}
	q.Add("Path", name)

	res, err := clientRequest(filesClient(c), "GET", "/files/download", q, nil)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	_, err = io.Copy(w, res.Body)

	return err
}

func mvCmd(c *instance.Instance, from, to string) error {
	q := url.Values{}
	q.Add("Path", from)
	doc, err := filesRequest(c, "GET", "/files/metadata", q, nil)
	if err != nil {
		return err
	}

	q = url.Values{}
	q.Add("Path", path.Dir(to))
	parent, err := filesRequest(c, "GET", "/files/metadata", q, nil)
	if err != nil {
		return err
	}

	q = url.Values{}
	q.Add("rev", doc.Data.Rev)

	body := &fileAPIPatch{}
	body.Data.Attrs = filePatch{
		Name:  path.Base(to),
		DirID: parent.Data.ID,
	}

	return clientRequestAndClose(filesClient(c), "PATCH", "/files/"+doc.Data.ID, q, body)
}

func rmCmd(c *instance.Instance, name string, force, recur bool) error {
	q := url.Values{}
	q.Add("Path", name)
	doc, err := filesRequest(c, "GET", "/files/metadata", q, nil)
	if err != nil {
		return err
	}

	if force {
		return fmt.Errorf("not implemented")
	}

	if !recur && len(doc.Included) > 0 {
		return fmt.Errorf("Directory is not empty")
	}

	return clientRequestAndClose(filesClient(c), "DELETE", "/files/"+doc.Data.ID, nil, nil)
}

func restoreCmd(c *instance.Instance, name string) error {
	q := url.Values{}
	q.Add("Path", name)
	doc, err := filesRequest(c, "GET", "/metadata", q, nil)
	if err != nil {
		return err
	}

	return clientRequestAndClose(filesClient(c), "POST", "/trash/"+doc.Data.ID, nil, nil)
}

func importFiles(c *instance.Instance, from, to string, match *regexp.Regexp) error {
	from = path.Clean(from)
	to = path.Clean(to)

	log.Infof("Importing from %s to cozy://%s", from, to)

	paths := make(map[string]string)

	mkdir := func(name string) error {
		q := url.Values{}
		q.Add("Path", name)
		q.Add("Type", "directory")
		q.Add("Recursive", "true")
		doc, err := filesRequest(c, "POST", "/files/", q, nil)
		if err != nil {
			return err
		}
		paths[name] = doc.Data.ID
		return nil
	}

	upload := func(localname, distname string) error {
		var err error

		dirname := path.Dir(distname)
		if dirname != string(os.PathSeparator) {
			err = mkdir(dirname)
			if err != nil {
				return err
			}
		}

		r, err := os.Open(localname)
		if err != nil {
			return err
		}
		defer r.Close()

		q := url.Values{}
		q.Add("Type", "file")
		q.Add("Name", path.Base(distname))

		dirID := paths[dirname]
		if dirID == "" {
			panic(fmt.Errorf("Missing directory %s", dirname))
		}

		req, err := clientCreateRequest(filesClient(c), "POST", "/files/"+dirID, q, r)
		if err != nil {
			return err
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		return clientErrCheck(res)
	}

	// TODO: symlinks ?
	return filepath.Walk(from, func(localname string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		isDir := f.IsDir()
		if localname == from && isDir {
			return nil
		}

		if match != nil && !match.MatchString(localname) {
			return nil
		}

		distname := path.Join(to, strings.Replace(localname, from, "", 1))
		if f.IsDir() {
			log.Debugln("create dir", distname)
			if !flagImportDryRun {
				return mkdir(distname)
			}
		} else {
			log.Debugf("copying file %s to %s", localname, distname)
			if !flagImportDryRun {
				return upload(localname, distname)
			}
		}

		return nil
	})
}

func getInstance(domain string) (*instance.Instance, error) {
	c, err := instance.Get(domain)
	if err != nil {
		if err == instance.ErrNotFound {
			err = fmt.Errorf("Could not find the cozy instance. Please use `instances add` command.")
		}
		return nil, err
	}
	return c, nil
}

func splitArgs(command string) []string {
	args := regexp.MustCompile("'.+'|\".+\"|\\S+").FindAllString(command, -1)
	for i, a := range args {
		l := len(a)
		switch {
		case a[0] == '\'' && a[l-1] == '\'':
			args[i] = strings.Trim(a, "'")
		case a[0] == '"' && a[l-1] == '"':
			args[i] = strings.Trim(a, "\"")
		}
	}
	return args
}

type fileData struct {
	ID    string            `json:"id"`
	Rev   string            `json:"rev"`
	Attrs *vfs.DirOrFileDoc `json:"attributes"`
}

type fileAPIData struct {
	Data     *fileData  `json:"data"`
	Included []fileData `json:"included"`
}

type filePatch struct {
	Name  string `json:"name"`
	DirID string `json:"dir_id"`
}

type fileAPIPatch struct {
	Data struct {
		Attrs filePatch `json:"attributes"`
	} `json:"data"`
}

func filesClient(c *instance.Instance) *client {
	return &client{addr: c.Domain}
}

func filesRequest(c *instance.Instance, method, path string, q url.Values, body interface{}) (*fileAPIData, error) {
	var doc fileAPIData
	err := clientRequestParsed(filesClient(c), method, path, q, body, &doc)
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

func init() {
	importFilesCmd.Flags().StringVar(&flagImportFrom, "from", "", "Directory to import from in cozy")
	importFilesCmd.Flags().StringVar(&flagImportTo, "to", "/", "Directory to import to in cozy")
	importFilesCmd.Flags().BoolVar(&flagImportDryRun, "dry-run", false, "Do not actually import the files")
	importFilesCmd.Flags().StringVar(&flagImportMatch, "match", "", "Pattern that the imported files must match")

	filesCmdGroup.AddCommand(execFilesCmd)
	filesCmdGroup.AddCommand(importFilesCmd)

	RootCmd.AddCommand(filesCmdGroup)
}
