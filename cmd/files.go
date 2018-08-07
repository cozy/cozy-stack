package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	humanize "github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var errFilesExec = errors.New("Bad usage of files exec")
var errFilesMissingDomain = errors.New("Missing --domain flag")

const filesExecUsage = `Available commands:

    mkdir <name>               Creates a directory with specified name
    ls [-l] [-a] [-h] <name>   Prints the children of the specified directory
    tree <name>                Prints the tree structure of the specified directory
    attrs <name>               Prints the attributes of the specified file or directory
    cat <name>                 Echo the file content in stdout
    mv <from> <to>             Rename a file or directory
    rm [-f] [-r] <name>        Move the file to trash, or delete it permanently with -f flag
    restore <name>             Restore a file or directory from trash

	Don't forget to put quotes around the command!
`

var flagFilesDomain string
var flagImportFrom string
var flagImportTo string
var flagImportDryRun bool
var flagImportMatch string

// filesCmdGroup represents the instances command
var filesCmdGroup = &cobra.Command{
	Use:   "files <command>",
	Short: "Interact with the cozy filesystem",
	Long: `
cozy-stack files allows to interact with the cozy filesystem.

It provides command to create, move copy or delete files and
directories inside your cozy instance, using the command line
interface. It also provide an import command to import from your
current filesystem into cozy.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var execFilesCmd = &cobra.Command{
	Use:   "exec [--domain domain] <command>",
	Short: "Execute the given command on the specified domain and leave",
	Long:  "Execute a command on the VFS of the specified domain.\n" + filesExecUsage,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}
		if flagFilesDomain == "" {
			errPrintfln("%s", errFilesMissingDomain)
			return cmd.Usage()
		}
		c := newClient(flagFilesDomain, consts.Files)
		command := args[0]
		err := execCommand(c, command, os.Stdout)
		if err == errFilesExec {
			return cmd.Usage()
		}
		return err
	},
}

var importFilesCmd = &cobra.Command{
	Use:   "import [--domain domain] [--match pattern] --from <name> --to <name>",
	Short: "Import the specified file or directory into cozy",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagFilesDomain == "" {
			errPrintfln("%s", errFilesMissingDomain)
			return cmd.Usage()
		}
		if flagImportFrom == "" || flagImportTo == "" {
			return cmd.Usage()
		}

		var match *regexp.Regexp
		if flagImportMatch != "" {
			var err error
			match, err = regexp.Compile(flagImportMatch)
			if err != nil {
				return err
			}
		}

		c := newClient(flagFilesDomain, consts.Files)
		return importFiles(c, flagImportFrom, flagImportTo, match)
	},
}

func execCommand(c *client.Client, command string, w io.Writer) error {
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

func mkdirCmd(c *client.Client, name string, mkdirP bool) error {
	var err error
	if mkdirP {
		_, err = c.Mkdirall(name)
	} else {
		_, err = c.Mkdir(name)
	}
	return err
}

func lsCmd(c *client.Client, root string, w io.Writer, verbose, human, all bool) error {
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

	root = path.Clean(root)

	err := c.WalkByPath(root, func(n string, doc *client.DirOrFile, err error) error {
		if err != nil {
			return err
		}
		if n == root {
			return nil
		}

		attrs := doc.Attrs
		var typ, name, size, mdate, exec string

		name = attrs.Name

		if now.Year() == attrs.UpdatedAt.Year() {
			mdate = attrs.UpdatedAt.Format("Jan 02 15:04")
		} else {
			mdate = attrs.UpdatedAt.Format("Jan 02 2015")
		}

		if attrs.Type == consts.DirType {
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
		if doc.Attrs.Type == client.DirType {
			return filepath.SkipDir
		}
		return nil
	})

	if err != nil {
		return err
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

func treeCmd(c *client.Client, root string, w io.Writer) error {
	root = path.Clean(root)

	return c.WalkByPath(root, func(name string, doc *client.DirOrFile, err error) error {
		if err != nil {
			return err
		}

		attrs := doc.Attrs
		if name == root {
			_, err = fmt.Fprintln(w, name)
			return err
		}

		level := strings.Count(strings.TrimPrefix(name, root)[1:], "/") + 1
		for i := 0; i < level; i++ {
			if i == level-1 {
				_, err = fmt.Fprintf(w, "└── ")
			} else {
				_, err = fmt.Fprintf(w, "|  ")
			}
			if err != nil {
				return err
			}
		}
		_, err = fmt.Fprintln(w, attrs.Name)
		return err
	})
}

func attrsCmd(c *client.Client, name string, w io.Writer) error {
	doc, err := c.GetDirOrFileByPath(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(doc)
}

func catCmd(c *client.Client, name string, w io.Writer) error {
	r, err := c.DownloadByPath(name)
	if err != nil {
		return err
	}

	defer r.Close()
	_, err = io.Copy(w, r)

	return err
}

func mvCmd(c *client.Client, from, to string) error {
	return c.Move(from, to)
}

func rmCmd(c *client.Client, name string, force, recur bool) error {
	if force {
		return fmt.Errorf("not implemented")
	}
	return c.TrashByPath(name)
}

func restoreCmd(c *client.Client, name string) error {
	return c.RestoreByPath(name)
}

type importer struct {
	c     *client.Client
	paths map[string]string
}

func (i *importer) mkdir(name string) (string, error) {
	doc, err := i.c.Mkdirall(name)
	if err != nil {
		return "", err
	}
	i.paths[name] = doc.ID
	return doc.ID, nil
}

func (i *importer) upload(localname, distname string) error {
	var err error

	dirname := path.Dir(distname)
	dirID, ok := i.paths[dirname]
	if !ok && dirname != string(os.PathSeparator) {
		dirID, err = i.mkdir(dirname)
		if err != nil {
			return err
		}
	}

	infos, err := os.Stat(localname)
	if err != nil {
		return err
	}

	r, err := os.Open(localname)
	if err != nil {
		return err
	}
	defer r.Close()

	_, err = i.c.Upload(&client.Upload{
		Name:          path.Base(distname),
		DirID:         dirID,
		Contents:      r,
		ContentLength: infos.Size(),
		ContentType:   mime.TypeByExtension(localname),
	})
	return err
}

func importFiles(c *client.Client, from, to string, match *regexp.Regexp) error {
	from = path.Clean(from)
	to = path.Clean(to)

	i := &importer{
		c:     c,
		paths: make(map[string]string),
	}

	fromInfos, err := os.Stat(from)
	if err != nil {
		return err
	}
	if !fromInfos.IsDir() {
		fmt.Printf("Importing file %s to cozy://%s\n", from, to)
		return i.upload(from, to)
	}

	fmt.Printf("Importing from %s to cozy://%s\n", from, to)

	// TODO: symlinks ?
	return filepath.Walk(from, func(localname string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if match != nil && !match.MatchString(localname) {
			return nil
		}

		if localname == from {
			if f.IsDir() {
				return nil
			}
			return fmt.Errorf("Not a directory: %s", localname)
		}

		distname := path.Join(to, strings.TrimPrefix(localname, from))
		if f.IsDir() {
			fmt.Printf("create dir %s\n", distname)
			if !flagImportDryRun {
				if _, err = i.mkdir(distname); err != nil {
					return err
				}
			}
		} else {
			fmt.Printf("copying file %s to %s\n", localname, distname)
			if !flagImportDryRun {
				return i.upload(localname, distname)
			}
		}

		return nil
	})
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

func init() {
	domain := os.Getenv("COZY_DOMAIN")
	if domain == "" && config.IsDevRelease() {
		domain = defaultDevDomain
	}

	filesCmdGroup.PersistentFlags().StringVar(&flagFilesDomain, "domain", domain, "specify the domain name of the instance")

	importFilesCmd.Flags().StringVar(&flagImportFrom, "from", "", "directory to import from in cozy")
	importFilesCmd.MarkFlagRequired("from")
	importFilesCmd.Flags().StringVar(&flagImportTo, "to", "/", "directory to import to in cozy")
	importFilesCmd.MarkFlagRequired("to")
	importFilesCmd.Flags().BoolVar(&flagImportDryRun, "dry-run", false, "do not actually import the files")
	importFilesCmd.Flags().StringVar(&flagImportMatch, "match", "", "pattern that the imported files must match")

	filesCmdGroup.AddCommand(execFilesCmd)
	filesCmdGroup.AddCommand(importFilesCmd)

	RootCmd.AddCommand(filesCmdGroup)
}
