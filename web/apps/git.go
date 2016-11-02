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

	"github.com/cozy/cozy-stack/vfs"
	"github.com/spf13/afero"
	git "gopkg.in/src-d/go-git.v4"
	gitSt "gopkg.in/src-d/go-git.v4/storage/filesystem"
	gitFS "gopkg.in/src-d/go-git.v4/utils/fs"
)

const manifestFilename = "manifest.webapp"
const githubRawManifestUrl = "https://raw.githubusercontent.com/%s/%s/%s/%s"

var githubURLRegex = regexp.MustCompile(`/([^/]+)/([^/]+).git`)

type GitClient struct {
	vfsC *vfs.Context
	src  string
}

func NewGitClient(vfsC *vfs.Context, rawurl string) *GitClient {
	return &GitClient{vfsC: vfsC, src: rawurl}
}

func (g *GitClient) FetchManifest() (io.ReadCloser, error) {
	src, err := url.Parse(g.src)
	if err != nil {
		return nil, err
	}

	if src.Host == "github.com" {
		return g.fetchManifestFromGithub(src)
	}

	// TODO
	return nil, errors.New("Not implemented")
}

func (g *GitClient) fetchManifestFromGithub(src *url.URL) (io.ReadCloser, error) {
	submatch := githubURLRegex.FindStringSubmatch(src.Path)
	if len(submatch) != 3 {
		return nil, &url.Error{"parsepath", src.String(), errors.New("Could not parse url git path")}
	}

	user, project := submatch[1], submatch[2]
	var branch string
	if src.Fragment != "" {
		branch = src.Fragment
	} else {
		branch = "master"
	}

	manURL := fmt.Sprintf(githubRawManifestUrl, user, project, branch, manifestFilename)
	resp, err := http.Get(manURL)
	if err != nil {
		return nil, ErrSourceNotReachable
	}

	if resp.StatusCode != 200 {
		return nil, ErrSourceNotReachable
	}

	return resp.Body, nil
}

func (g *GitClient) Fetch(appdir string) error {
	gfs := newGFS(g.vfsC, appdir)
	storage, err := gitSt.NewStorage(gfs)
	if err != nil {
		return err
	}

	rep, err := git.NewRepository(storage)
	if err != nil {
		return err
	}

	src, err := url.Parse(g.src)
	if err != nil {
		return err
	}

	// go-git does not support git protocol. we switch to https silently.
	if src.Scheme == "git" {
		src.Scheme = "https"
	}

	err = rep.Clone(&git.CloneOptions{
		URL:   src.String(),
		Depth: 1,
	})
	if err != nil {
		return err
	}

	ref, err := rep.Head()
	if err != nil {
		return err
	}

	commit, err := rep.Commit(ref.Hash())
	if err != nil {
		return err
	}

	files, err := commit.Files()
	if err != nil {
		return err
	}

	return files.ForEach(func(f *git.File) (err error) {
		abs := path.Join(gfs.Base(), f.Name)
		dir := path.Dir(abs)

		err = g.vfsC.MkdirAll(dir)
		if err != nil {
			return
		}

		file, err := g.vfsC.Create(abs)
		if err != nil {
			return
		}

		defer func() {
			if cerr := file.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()

		r, err := f.Reader()
		if err != nil {
			return
		}

		defer r.Close()
		_, err = io.Copy(file, r)

		return
	})
}

type gfs struct {
	vfsC *vfs.Context
	base string
	dir  *vfs.DirDoc
}

type gfileRead struct {
	f      afero.File
	name   string
	closed bool
}

type gfileWrite struct {
	f      io.WriteCloser
	name   string
	closed bool
}

func newGFileRead(f afero.File, name string) *gfileRead {
	return &gfileRead{
		f:      f,
		name:   name,
		closed: false,
	}
}

func (f *gfileRead) Filename() string {
	return f.name
}

func (f *gfileRead) IsClosed() bool {
	return f.closed
}

func (f *gfileRead) Write(p []byte) (n int, err error) {
	return 0, os.ErrInvalid
}

func (f *gfileRead) Read(p []byte) (n int, err error) {
	return f.f.Read(p)
}

func (f *gfileRead) Seek(offset int64, whence int) (int64, error) {
	return f.f.Seek(offset, whence)
}

func (f *gfileRead) Close() error {
	f.closed = true
	return f.f.Close()
}

func newGFileWrite(f io.WriteCloser, name string) *gfileWrite {
	return &gfileWrite{
		f:      f,
		name:   name,
		closed: false,
	}
}

func (f *gfileWrite) Filename() string {
	return f.name
}

func (f *gfileWrite) IsClosed() bool {
	return f.closed
}

func (f *gfileWrite) Write(p []byte) (n int, err error) {
	return f.f.Write(p)
}

func (f *gfileWrite) Read(p []byte) (n int, err error) {
	return 0, os.ErrInvalid
}

func (f *gfileWrite) Seek(offset int64, whence int) (int64, error) {
	return 0, os.ErrInvalid
}

func (f *gfileWrite) Close() error {
	f.closed = true
	return f.f.Close()
}

func newGFS(vfsC *vfs.Context, base string) *gfs {
	dir, err := vfs.GetDirDocFromPath(vfsC, base, false)
	if err != nil {
		panic(err)
	}

	return &gfs{
		vfsC: vfsC,
		base: path.Clean(base),
		dir:  dir,
	}
}

func (fs *gfs) createFile(fullpath, filename string) (*gfileWrite, error) {
	var err error

	var dirbase = path.Dir(fullpath)
	if err = fs.vfsC.MkdirAll(dirbase); err != nil {
		return nil, err
	}

	file, err := fs.vfsC.Create(fullpath)
	if err != nil {
		return nil, err
	}

	return newGFileWrite(file, filename), nil
}

func (fs *gfs) Create(filename string) (gitFS.File, error) {
	return fs.createFile(fs.Join(fs.base, filename), filename)
}

func (fs *gfs) Open(filename string) (gitFS.File, error) {
	fullpath := fs.Join(fs.base, filename)
	f, err := fs.vfsC.Open(fullpath)
	if err != nil {
		return nil, err
	}
	return newGFileRead(f, fullpath[len(fs.base)+1:]), nil
}

func (fs *gfs) Stat(filename string) (gitFS.FileInfo, error) {
	return fs.vfsC.Stat(fs.Join(fs.base, filename))
}

func (fs *gfs) ReadDir(dirname string) ([]gitFS.FileInfo, error) {
	l, err := fs.vfsC.ReadDir(fs.Join(fs.base, dirname))
	if err != nil {
		return nil, err
	}

	var s = make([]gitFS.FileInfo, len(l))
	for i, f := range l {
		s[i] = f
	}

	return s, nil
}

func (fs *gfs) TempFile(dirname, prefix string) (gitFS.File, error) {
	// TODO: not really robust tempfile...
	filename := fs.Join("/", dirname, prefix+"_"+strconv.Itoa(int(time.Now().UnixNano())))
	fullpath := fs.Join(fs.base, filename)
	return fs.createFile(fullpath, filename)
}

func (fs *gfs) Rename(from, to string) error {
	return fs.vfsC.Rename(fs.Join(fs.base, from), fs.Join(fs.base, to))
}

func (fs *gfs) Join(elem ...string) string {
	return path.Join(elem...)
}

func (fs *gfs) Dir(name string) gitFS.Filesystem {
	return newGFS(fs.vfsC, fs.Join(fs.base, name))
}

func (fs *gfs) Base() string {
	return fs.base
}

var (
	_ Client           = &GitClient{}
	_ gitFS.Filesystem = &gfs{}
	_ gitFS.File       = &gfileWrite{}
	_ gitFS.File       = &gfileRead{}
)
