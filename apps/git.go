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
	git "gopkg.in/src-d/go-git.v4"
	gitSt "gopkg.in/src-d/go-git.v4/storage/filesystem"
	gitFS "gopkg.in/src-d/go-git.v4/utils/fs"
)

const githubRawManifestURL = "https://raw.githubusercontent.com/%s/%s/%s/%s"

var githubURLRegex = regexp.MustCompile(`/([^/]+)/([^/]+).git`)

type gitClient struct {
	vfsC vfs.Context
	src  string
}

func newGitClient(vfsC vfs.Context, rawurl string) *gitClient {
	return &gitClient{vfsC: vfsC, src: rawurl}
}

func (g *gitClient) FetchManifest() (io.ReadCloser, error) {
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

func (g *gitClient) fetchManifestFromGithub(src *url.URL) (io.ReadCloser, error) {
	submatch := githubURLRegex.FindStringSubmatch(src.Path)
	if len(submatch) != 3 {
		return nil, &url.Error{
			Op:  "parsepath",
			URL: src.String(),
			Err: errors.New("Could not parse url git path"),
		}
	}

	user, project := submatch[1], submatch[2]
	var branch string
	if src.Fragment != "" {
		branch = src.Fragment
	} else {
		branch = "master"
	}

	manURL := fmt.Sprintf(githubRawManifestURL, user, project, branch, ManifestFilename)
	resp, err := http.Get(manURL)
	if err != nil {
		return nil, ErrManifestNotReachable
	}

	if resp.StatusCode != 200 {
		return nil, ErrManifestNotReachable
	}

	return resp.Body, nil
}

func (g *gitClient) Fetch(vfsC vfs.Context, appdir string) error {
	gitdir := path.Join(appdir, ".git")
	err := vfs.Mkdir(vfsC, gitdir)
	if err != nil {
		return err
	}

	gfs := newGFS(vfsC, gitdir)
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
		abs := path.Join(appdir, f.Name)
		dir := path.Dir(abs)

		err = vfs.MkdirAll(vfsC, dir)
		if err != nil {
			return
		}

		file, err := vfs.Create(vfsC, abs)
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
	vfsC vfs.Context
	base string
	dir  *vfs.DirDoc
}

type gfile struct {
	f      *vfs.File
	name   string
	closed bool
}

func newGFile(f *vfs.File, name string) *gfile {
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

func (f *gfile) Read(p []byte) (n int, err error) {
	return f.f.Read(p)
}

func (f *gfile) Write(p []byte) (n int, err error) {
	return f.f.Write(p)
}

func (f *gfile) Seek(offset int64, whence int) (int64, error) {
	return f.f.Seek(offset, whence)
}

func (f *gfile) Close() error {
	f.closed = true
	return f.f.Close()
}

func newGFS(vfsC vfs.Context, base string) *gfs {
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

func (fs *gfs) OpenFile(name string, flag int, perm os.FileMode) (gitFS.File, error) {
	var err error

	fullpath := path.Join(fs.base, name)
	dirbase := path.Dir(fullpath)

	if flag&os.O_CREATE != 0 {
		if err = vfs.MkdirAll(fs.vfsC, dirbase); err != nil {
			return nil, err
		}
	}

	file, err := vfs.OpenFile(fs.vfsC, fullpath, flag, perm)
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
	f, err := vfs.OpenFile(fs.vfsC, fullpath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return newGFile(f, fullpath[len(fs.base)+1:]), nil
}

func (fs *gfs) Remove(name string) error {
	return vfs.Remove(fs.vfsC, fs.Join(fs.base, name))
}

func (fs *gfs) Stat(name string) (gitFS.FileInfo, error) {
	return vfs.Stat(fs.vfsC, fs.Join(fs.base, name))
}

func (fs *gfs) ReadDir(name string) ([]gitFS.FileInfo, error) {
	l, err := vfs.ReadDir(fs.vfsC, fs.Join(fs.base, name))
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
	return vfs.Rename(fs.vfsC, fs.Join(fs.base, from), fs.Join(fs.base, to))
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
	_ Client           = &gitClient{}
	_ gitFS.Filesystem = &gfs{}
	_ gitFS.File       = &gfile{}
)
