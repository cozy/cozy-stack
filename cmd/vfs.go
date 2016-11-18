package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var errVfsExec = errors.New("errVfsExec")

const vfsExecUsage = `Available commands:

  mkdir [name]               Creates a directory with specified name
  touch [name]               Creates or change a file modifications time
  ls [-l] [-a] [-h] [name]   Prints the children of the specified directory
  tree [name]                Prints the tree structure of the specified directory
  attrs [name]               Prints the attributes of the specified file or directory
  cat [name]                 Echo the file content in stdout
  mv [from] [to]             Rename a file or directory
  rm [-f] [-r] [name]        Move the file to trash, or delete it permanently with -f flag
`

var flagImportFrom string
var flagImportTo string
var flagImportDryRun bool
var flagImportMatch string

// vfsCmdGroup represents the instances command
var vfsCmdGroup = &cobra.Command{
	Use:   "vfs [command]",
	Short: "Interact with the cozy filesystem",
	Long: `
cozy-stack vfs allows to interact with the cozy filesystem.

It provides command to create, move copy or delete files and
directories inside your cozy instance, using the command line
interface. It also provide an import command to import from your
current filesystem into cozy.
`,
	Run: func(cmd *cobra.Command, args []string) { cmd.Help() },
}

var execVfsCmd = &cobra.Command{
	Use:   "exec [domain] [command]",
	Short: "Execute the given command on the specified domain and leave",
	Long:  "Execute a command on the VFS of the specified domain.\n" + vfsExecUsage,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		conn, err := net.Dial("tcp", config.CmdAddr())
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(conn, "%s\n%s\n\n", args[0], strings.Join(args[1:], " "))
		if err != nil {
			return err
		}

		_, err = io.Copy(os.Stdout, conn)
		return err
	},
}

var importVfsCmd = &cobra.Command{
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

		return vfsImport(c, flagImportFrom, flagImportTo, match)
	},
}

// vfsListenAndServe starts a tcp listener and wait for new clients to
// a command.
//
// The message exchange is rudimentary for now, and not secure. The
// client first needs to send the domain followed by a newline then
// its command followed by a newline.  Commands are not multiplexed
// for now, only one command can be executed for one tcp connection.
func vfsListenAndServe() {
	l, err := net.Listen("tcp", config.CmdAddr())
	if err != nil {
		log.Error(err)
		return
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error(err)
			return
		}

		go func() {
			errc := newConn(conn)
			if errc != nil {
				log.Error(errc)
			}
		}()
	}
}

func newConn(conn net.Conn) (err error) {
	defer func() {
		if err == errVfsExec {
			_, err = fmt.Fprintln(conn, vfsExecUsage)
		}
		if err != nil && err != io.ErrClosedPipe && err != io.ErrShortWrite {
			_, err = fmt.Fprintln(conn, err)
		}
		if errc := conn.Close(); errc != nil && err == nil {
			err = errc
		}
	}()

	scanner := bufio.NewScanner(conn)

	if !scanner.Scan() {
		return scanner.Err()
	}

	c, err := getInstance(scanner.Text())
	if err != nil {
		return err
	}

	if !scanner.Scan() {
		return scanner.Err()
	}

	return execCommand(c, scanner.Text(), conn)
}

func execCommand(c *instance.Instance, arg string, w io.Writer) error {
	args := strings.Fields(arg)
	if len(args) == 0 {
		return errVfsExec
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
		return errVfsExec
	}

	switch cmdname {
	case "mkdir":
		return vfsMkdirCmd(c, args[0], flagMkdirP)
	case "touch":
		return vfsTouchCmd(c, args[0])
	case "ls":
		return vfsLsCmd(c, args[0], w, flagLsVerbose, flagLsHuman, flagLsAll)
	case "tree":
		return vfsTreeCmd(c, args[0], w)
	case "attrs":
		return vfsAttrsCmd(c, args[0], w)
	case "cat":
		return vfsCatCmd(c, args[0], w)
	case "mv":
		if len(args) < 2 {
			return errVfsExec
		}
		return vfsMvCmd(c, args[0], args[1])
	case "rm":
		return vfsRmCmd(c, args[0], flagRmForce, flagRmRecur)
	}

	return errVfsExec
}

func vfsMkdirCmd(c *instance.Instance, name string, mkdirP bool) error {
	if mkdirP {
		return vfs.MkdirAll(c, name)
	}
	return vfs.Mkdir(c, name)
}

func vfsTouchCmd(c *instance.Instance, name string) error {
	return vfs.Touch(c, name)
}

func vfsLsCmd(c *instance.Instance, root string, w io.Writer, verbose, human, all bool) error {
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

	err := vfs.Walk(c, root, func(path string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}
		if path == root && dir != nil {
			return nil
		}

		var typ, name, size, mdate, exec string
		if dir != nil {
			name = dir.Name
			typ = "d"
			if now.Year() == dir.UpdatedAt.Year() {
				mdate = dir.UpdatedAt.Format("Jan 02 15:04")
			} else {
				mdate = dir.UpdatedAt.Format("Jan 02 2015")
			}
			exec = "x"
		} else {
			name = file.Name
			typ = ""
			if now.Year() == file.UpdatedAt.Year() {
				mdate = file.UpdatedAt.Format("Jan 02 15:04")
			} else {
				mdate = file.UpdatedAt.Format("Jan 02 2015")
			}
			if file.Executable {
				exec = "x"
			} else {
				exec = "-"
			}
			if human {
				size = humanize.Bytes(uint64(file.Size))
			} else {
				size = humanize.Comma(file.Size)
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

		if dir != nil {
			return vfs.ErrSkipDir
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

func vfsTreeCmd(c *instance.Instance, root string, w io.Writer) error {
	return vfs.Walk(c, root, func(name string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}
		if name == root && dir != nil {
			return nil
		}
		relname := strings.Replace(name, root+string(os.PathSeparator), "", 1)
		level := len(strings.Split("/"+relname, "/"))
		for i := 0; i < level-1; i++ {
			if i == level-2 {
				_, err = fmt.Fprintf(w, "└── ")
			} else {
				_, err = fmt.Fprintf(w, "|  ")
			}
			if err != nil {
				return err
			}
		}
		_, err = fmt.Fprintln(w, path.Base(name))
		return err
	})
}

func vfsAttrsCmd(c *instance.Instance, name string, w io.Writer) error {
	dir, file, err := vfs.GetDirOrFileDocFromPath(c, name, false)
	if err != nil {
		return err
	}

	var data []byte
	if dir != nil {
		data, err = json.MarshalIndent(dir, "", "\t")
	} else {
		data, err = json.MarshalIndent(file, "", "\t")
	}
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func vfsCatCmd(c *instance.Instance, name string, w io.Writer) error {
	f, err := vfs.OpenFile(c, name, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	if errc := f.Close(); errc != nil && err == nil {
		return errc
	}
	return err
}

func vfsMvCmd(c *instance.Instance, from, to string) error {
	return vfs.Rename(c, from, to)
}

func vfsRmCmd(c *instance.Instance, name string, force, recur bool) (err error) {
	if !force {
		err = vfs.Trash(c, name, recur)
	} else {
		err = fmt.Errorf("not implemented")
	}
	return
}

func vfsImport(c *instance.Instance, from, to string, match *regexp.Regexp) error {
	from = path.Clean(from)
	to = path.Clean(to)

	log.Infof("Importing from %s to cozy://%s", from, to)

	// TODO: support symlinks
	err := filepath.Walk(from, func(name string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		isDir := f.IsDir()
		if name == from && isDir {
			return nil
		}

		relname := path.Join(to, strings.Replace(name, from, "", 1))

		if match != nil && !match.MatchString(relname) {
			return nil
		}

		if f.IsDir() {
			log.Debugln("create dir", relname)
			if !flagImportDryRun {
				return vfs.MkdirAll(c, relname)
			}
			return nil
		}

		log.Debugf("copying file %s to %s", name, relname)
		if flagImportDryRun {
			return nil
		}

		dirname := path.Dir(relname)
		if dirname != string(os.PathSeparator) {
			err = vfs.MkdirAll(c, dirname)
			if err != nil {
				return err
			}
		}

		r, err := os.Open(name)
		if err != nil {
			return err
		}

		w, err := vfs.OpenFile(c, relname, os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.Mode().Perm())
		if err != nil {
			return err
		}
		defer w.Close()

		_, err = io.Copy(w, r)
		return err
	})

	return err
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

func init() {
	importVfsCmd.Flags().StringVar(&flagImportFrom, "from", "", "Directory to import from in cozy")
	importVfsCmd.Flags().StringVar(&flagImportTo, "to", "/", "Directory to import to in cozy")
	importVfsCmd.Flags().BoolVar(&flagImportDryRun, "dry-run", false, "Do not actually import the files")
	importVfsCmd.Flags().StringVar(&flagImportMatch, "match", "", "Pattern that the imported files must match")

	vfsCmdGroup.AddCommand(execVfsCmd)
	vfsCmdGroup.AddCommand(importVfsCmd)

	RootCmd.AddCommand(vfsCmdGroup)
}
