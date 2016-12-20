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

	"github.com/cozy/cozy-stack/pkg/vfs"
	git "gopkg.in/src-d/go-git.v4"
	gitObj "gopkg.in/src-d/go-git.v4/plumbing/object"
	gitSt "gopkg.in/src-d/go-git.v4/storage/filesystem"
	gitFS "srcd.works/go-billy.v1"
)

const ghRawManifestURL = "https://raw.githubusercontent.com/%s/%s/%s/%s"

// ghURLRegex is used to identify github
var ghURLRegex = regexp.MustCompile(`/([^/]+)/([^/]+).git`)

type gitFetcher struct {
	ctx vfs.Context
}

func newGitFetcher(ctx vfs.Context) *gitFetcher {
	return &gitFetcher{ctx: ctx}
}

func (g *gitFetcher) FetchManifest(src *url.URL) (io.ReadCloser, error) {
	var err error

	var u string
	if src.Host == "github.com" {
		u, err = resolveGithubURL(src)
	} else {
		u, err = resolveManifestURL(src)
	}
	if err != nil {
		return nil, err
	}

	res, err := http.Get(u)
	if err != nil || res.StatusCode != 200 {
		return nil, ErrManifestNotReachable
	}

	return res.Body, nil
}

func (g *gitFetcher) Fetch(src *url.URL, appdir string) error {
	ctx := g.ctx

	gitdir := path.Join(appdir, ".git")
	_, err := vfs.Mkdir(ctx, gitdir, nil)
	if os.IsExist(err) {
		return g.pull(appdir, gitdir, src)
	}
	if err != nil {
		return err
	}

	return g.clone(appdir, gitdir, src)
}

// clone creates a new bare git repository and install all the files of the
// last commit in the application tree.
func (g *gitFetcher) clone(appdir, gitdir string, src *url.URL) error {
	ctx := g.ctx

	storage, err := gitSt.NewStorage(newGFS(ctx, gitdir))
	if err != nil {
		return err
	}

	rep, err := git.NewRepository(storage)
	if err != nil {
		return err
	}

	err = rep.Clone(&git.CloneOptions{
		URL:   src.String(),
		Depth: 1,
	})
	if err != nil {
		return err
	}

	return g.copyFiles(appdir, rep)
}

// pull will fetch the latest objects from the default remote and if updates
// are available, it will update the application tree files.
func (g *gitFetcher) pull(appdir, gitdir string, src *url.URL) error {
	ctx := g.ctx

	storage, err := gitSt.NewStorage(newGFS(ctx, gitdir))
	if err != nil {
		return err
	}

	rep, err := git.NewRepository(storage)
	if err != nil {
		return err
	}

	err = rep.Pull(&git.PullOptions{})
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}
	if err != nil {
		return err
	}

	// TODO: permanently remove application files instead of moving them to the
	// trash
	err = vfs.Walk(ctx, appdir, func(name string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}

		if name == appdir {
			return nil
		}
		if name == gitdir {
			return vfs.ErrSkipDir
		}

		if dir != nil {
			_, err = vfs.TrashDir(ctx, dir)
		} else {
			_, err = vfs.TrashFile(ctx, file)
		}
		if err != nil {
			return err
		}
		if dir != nil {
			return vfs.ErrSkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}

	return g.copyFiles(appdir, rep)
}

func (g *gitFetcher) copyFiles(appdir string, rep *git.Repository) error {
	ctx := g.ctx

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

	return files.ForEach(func(f *gitObj.File) (err error) {
		abs := path.Join(appdir, f.Name)
		dir := path.Dir(abs)

		_, err = vfs.MkdirAll(ctx, dir, nil)
		if err != nil {
			return
		}

		file, err := vfs.Create(ctx, abs)
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

func resolveGithubURL(src *url.URL) (string, error) {
	match := ghURLRegex.FindStringSubmatch(src.Path)
	if len(match) != 3 {
		return "", &url.Error{
			Op:  "parsepath",
			URL: src.String(),
			Err: errors.New("Could not parse url git path"),
		}
	}

	user, project := match[1], match[2]
	var branch string
	if src.Fragment != "" {
		branch = src.Fragment
	} else {
		branch = "master"
	}

	u := fmt.Sprintf(ghRawManifestURL, user, project, branch, ManifestFilename)
	return u, nil
}

func resolveManifestURL(src *url.URL) (string, error) {
	srccopy, _ := url.Parse(src.String())
	srccopy.Scheme = "http"
	if srccopy.Path[len(srccopy.Path)-1] != '/' {
		srccopy.Path += "/"
	}
	srccopy.Path = srccopy.Path + ManifestFilename
	return srccopy.String(), nil
}

type gfs struct {
	ctx  vfs.Context
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

func newGFS(ctx vfs.Context, base string) *gfs {
	dir, err := vfs.GetDirDocFromPath(ctx, base, false)
	if err != nil {
		panic(err)
	}

	return &gfs{
		ctx:  ctx,
		base: path.Clean(base),
		dir:  dir,
	}
}

func (fs *gfs) OpenFile(name string, flag int, perm os.FileMode) (gitFS.File, error) {
	var err error

	fullpath := path.Join(fs.base, name)
	dirbase := path.Dir(fullpath)

	if flag&os.O_CREATE != 0 {
		if _, err = vfs.MkdirAll(fs.ctx, dirbase, nil); err != nil {
			return nil, err
		}
	}

	file, err := vfs.OpenFile(fs.ctx, fullpath, flag, perm)
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
	f, err := vfs.OpenFile(fs.ctx, fullpath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return newGFile(f, fullpath[len(fs.base)+1:]), nil
}

func (fs *gfs) Remove(name string) error {
	return vfs.Remove(fs.ctx, fs.Join(fs.base, name))
}

func (fs *gfs) Stat(name string) (gitFS.FileInfo, error) {
	return vfs.Stat(fs.ctx, fs.Join(fs.base, name))
}

func (fs *gfs) ReadDir(name string) ([]gitFS.FileInfo, error) {
	l, err := vfs.ReadDir(fs.ctx, fs.Join(fs.base, name))
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
	return vfs.Rename(fs.ctx, fs.Join(fs.base, from), fs.Join(fs.base, to))
}

func (fs *gfs) Join(elem ...string) string {
	return path.Join(elem...)
}

func (fs *gfs) Dir(name string) gitFS.Filesystem {
	return newGFS(fs.ctx, fs.Join(fs.base, name))
}

func (fs *gfs) Base() string {
	return fs.base
}

var (
	_ Fetcher          = &gitFetcher{}
	_ gitFS.Filesystem = &gfs{}
	_ gitFS.File       = &gfile{}
)
