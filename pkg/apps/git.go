package apps

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/vfs"
	gitFS "gopkg.in/src-d/go-billy.v2"
	git "gopkg.in/src-d/go-git.v4"
	gitPlumbing "gopkg.in/src-d/go-git.v4/plumbing"
	gitObject "gopkg.in/src-d/go-git.v4/plumbing/object"
	gitStorage "gopkg.in/src-d/go-git.v4/storage/filesystem"
)

const ghRawManifestURL = "https://raw.githubusercontent.com/%s/%s/%s/%s"

// ghURLRegex is used to identify github
var ghURLRegex = regexp.MustCompile(`/([^/]+)/([^/]+).git`)

type gitFetcher struct {
	fs          vfs.VFS
	manFilename string
}

func newGitFetcher(fs vfs.VFS, manFilename string) *gitFetcher {
	return &gitFetcher{fs: fs, manFilename: manFilename}
}

var manifestClient = &http.Client{
	Timeout: 60 * time.Second,
}

func (g *gitFetcher) FetchManifest(src *url.URL) (io.ReadCloser, error) {
	var err error

	var u string
	if src.Host == "github.com" {
		u, err = resolveGithubURL(src, g.manFilename)
	} else {
		u, err = resolveManifestURL(src, g.manFilename)
	}
	if err != nil {
		return nil, err
	}

	res, err := manifestClient.Get(u)
	if err != nil || res.StatusCode != 200 {
		return nil, ErrManifestNotReachable
	}

	return res.Body, nil
}

func (g *gitFetcher) Fetch(src *url.URL, baseDir *vfs.DirDoc) error {
	log.Debugf("[git] Fetch %s", src.String())
	fs := g.fs

	gitDirName := path.Join(baseDir.Fullpath, ".git")
	gitDir, err := fs.DirByPath(gitDirName)
	if err == nil {
		return g.pull(baseDir, gitDir, src)
	}
	if !os.IsNotExist(err) {
		return err
	}
	gitDir, err = vfs.Mkdir(fs, gitDirName, nil)
	if err != nil {
		return err
	}
	return g.clone(baseDir, gitDir, src)
}

func getBranch(src *url.URL) string {
	if src.Fragment != "" {
		return "refs/heads/" + src.Fragment
	}
	return "HEAD"
}

// clone creates a new bare git repository and install all the files of the
// last commit in the application tree.
func (g *gitFetcher) clone(baseDir, gitDir *vfs.DirDoc, src *url.URL) error {
	fs := g.fs

	storage, err := gitStorage.NewStorage(newGFS(fs, gitDir))
	if err != nil {
		return err
	}

	branch := getBranch(src)
	log.Debugf("[git] Clone %s %s", src.String(), branch)

	rep, err := git.Clone(storage, nil, &git.CloneOptions{
		URL:           src.String(),
		Depth:         1,
		SingleBranch:  true,
		ReferenceName: gitPlumbing.ReferenceName(branch),
	})
	if err != nil {
		return err
	}

	return g.copyFiles(baseDir, rep)
}

// pull will fetch the latest objects from the default remote and if updates
// are available, it will update the application tree files.
func (g *gitFetcher) pull(baseDir, gitDir *vfs.DirDoc, src *url.URL) error {
	fs := g.fs

	storage, err := gitStorage.NewStorage(newGFS(fs, gitDir))
	if err != nil {
		return err
	}

	rep, err := git.Open(storage, nil)
	if err != nil {
		return err
	}

	branch := getBranch(src)
	log.Debugf("[git] Pull %s %s", src.String(), branch)

	err = rep.Pull(&git.PullOptions{
		SingleBranch:  true,
		ReferenceName: gitPlumbing.ReferenceName(branch),
	})
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}
	if err != nil {
		return err
	}

	iter := fs.DirIterator(baseDir, nil)
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if err != nil {
			return err
		}
		if d != nil && d.DocName == ".git" {
			continue
		}
		if d != nil {
			err = fs.DestroyDirAndContent(d)
		} else {
			err = fs.DestroyFile(f)
		}
		if err != nil {
			return err
		}
	}

	return g.copyFiles(baseDir, rep)
}

func (g *gitFetcher) copyFiles(baseDir *vfs.DirDoc, rep *git.Repository) error {
	fs := g.fs

	ref, err := rep.Head()
	if err != nil {
		return err
	}

	commit, err := rep.CommitObject(ref.Hash())
	if err != nil {
		return err
	}

	files, err := commit.Files()
	if err != nil {
		return err
	}

	return files.ForEach(func(f *gitObject.File) error {
		abs := path.Join(baseDir.Fullpath, f.Name)
		dir := path.Dir(abs)

		_, err := vfs.MkdirAll(fs, dir, nil)
		if err != nil {
			return err
		}

		file, err := vfs.Create(fs, abs)
		if err != nil {
			return err
		}

		defer func() {
			if cerr := file.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()

		r, err := f.Reader()
		if err != nil {
			return err
		}

		defer r.Close()
		_, err = io.Copy(file, r)

		return err
	})
}

func resolveGithubURL(src *url.URL, filename string) (string, error) {
	match := ghURLRegex.FindStringSubmatch(src.Path)
	if len(match) != 3 {
		return "", &url.Error{
			Op:  "parsepath",
			URL: src.String(),
			Err: errors.New("Could not parse url git path"),
		}
	}

	user, project := match[1], match[2]
	branch := "HEAD"
	if src.Fragment != "" {
		branch = src.Fragment
	}

	u := fmt.Sprintf(ghRawManifestURL, user, project, branch, filename)
	return u, nil
}

func resolveManifestURL(src *url.URL, filename string) (string, error) {
	// TODO check that it works with a branch
	srccopy, _ := url.Parse(src.String())
	srccopy.Scheme = "http"
	if srccopy.Path == "" || srccopy.Path[len(srccopy.Path)-1] != '/' {
		srccopy.Path += "/"
	}
	srccopy.Path = srccopy.Path + filename
	return srccopy.String(), nil
}

type gfs struct {
	fs   vfs.VFS
	base string
	dir  *vfs.DirDoc
}

type gfile struct {
	f      vfs.File
	name   string
	closed bool
}

func newGFile(f vfs.File, name string) *gfile {
	return &gfile{
		f:      f,
		name:   name,
		closed: false,
	}
}

func (f *gfile) Filename() string {
	return f.name
}

func (f *gfile) IsClosed() bool {
	return f.closed
}

func (f *gfile) Read(p []byte) (int, error) {
	return f.f.Read(p)
}

func (f *gfile) Write(p []byte) (int, error) {
	return f.f.Write(p)
}

func (f *gfile) Seek(offset int64, whence int) (int64, error) {
	return f.f.Seek(offset, whence)
}

func (f *gfile) Close() error {
	f.closed = true
	return f.f.Close()
}

func newGFS(fs vfs.VFS, base *vfs.DirDoc) *gfs {
	return &gfs{
		fs:   fs,
		base: path.Clean(base.Fullpath),
		dir:  base,
	}
}

func (fs *gfs) OpenFile(name string, flag int, perm os.FileMode) (gitFS.File, error) {
	var err error

	fullpath := path.Join(fs.base, name)
	dirbase := path.Dir(fullpath)

	if flag&os.O_CREATE != 0 {
		if _, err = vfs.MkdirAll(fs.fs, dirbase, nil); err != nil {
			return nil, err
		}
	}

	file, err := vfs.OpenFile(fs.fs, fullpath, flag, perm)
	if err != nil {
		return nil, err
	}

	return newGFile(file, name), nil
}

func (fs *gfs) Create(name string) (gitFS.File, error) {
	return fs.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_TRUNC, 0666)
}

func (fs *gfs) Open(name string) (gitFS.File, error) {
	fullpath := fs.Join(fs.base, name)
	f, err := vfs.OpenFile(fs.fs, fullpath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return newGFile(f, fullpath[len(fs.base)+1:]), nil
}

func (fs *gfs) Remove(name string) error {
	return vfs.Remove(fs.fs, fs.Join(fs.base, name))
}

func (fs *gfs) Stat(name string) (gitFS.FileInfo, error) {
	return vfs.Stat(fs.fs, fs.Join(fs.base, name))
}

func (fs *gfs) ReadDir(name string) ([]gitFS.FileInfo, error) {
	var s []gitFS.FileInfo
	dir, err := fs.fs.DirByPath(fs.Join(fs.base, name))
	if err != nil {
		return nil, err
	}
	iter := fs.fs.DirIterator(dir, nil)
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if err != nil {
			return nil, err
		}
		if d != nil {
			s = append(s, d)
		} else {
			s = append(s, f)
		}
	}
	return s, nil
}

func (fs *gfs) MkdirAll(path string, perm os.FileMode) error {
	_, err := vfs.MkdirAll(fs.fs, fs.Join(fs.base, path), nil)
	return err
}

func (fs *gfs) TempFile(dirname, prefix string) (gitFS.File, error) {
	// TODO: not really robust tempfile...
	name := fs.Join("/", dirname, prefix+"_"+strconv.Itoa(int(time.Now().UnixNano())))
	file, err := fs.Create(name)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return fs.OpenFile(name, os.O_WRONLY|os.O_TRUNC, 0666)
}

func (fs *gfs) Rename(from, to string) error {
	return vfs.Rename(fs.fs, fs.Join(fs.base, from), fs.Join(fs.base, to))
}

func (fs *gfs) Join(elem ...string) string {
	return path.Join(elem...)
}

func (fs *gfs) Dir(name string) gitFS.Filesystem {
	name = fs.Join(fs.base, name)
	dir, err := fs.fs.DirByPath(name)
	if err != nil {
		// FIXME https://issues.apache.org/jira/browse/COUCHDB-3336
		// With a cluster of couchdb, we can have a race condition where we
		// query an index before it has been updated for a directory that has
		// just been created.
		time.Sleep(1 * time.Second)
		dir, err = fs.fs.DirByPath(name)
		if err != nil {
			panic(err)
		}
	}
	return newGFS(fs.fs, dir)
}

func (fs *gfs) Base() string {
	return fs.base
}

var (
	_ Fetcher          = &gitFetcher{}
	_ gitFS.Filesystem = &gfs{}
	_ gitFS.File       = &gfile{}
)
