package apps

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/afero"
	gitFS "gopkg.in/src-d/go-billy.v2"
	git "gopkg.in/src-d/go-git.v4"
	gitPlumbing "gopkg.in/src-d/go-git.v4/plumbing"
	gitObject "gopkg.in/src-d/go-git.v4/plumbing/object"
	gitStorage "gopkg.in/src-d/go-git.v4/storage/filesystem"
)

// ghURLRegex is used to identify github
var ghURLRegex = regexp.MustCompile(`/([^/]+)/([^/]+).git`)

const ghRawManifestURL = "https://raw.githubusercontent.com/%s/%s/%s/%s"

// glURLRegex is used to identify gitlab
var glURLRegex = regexp.MustCompile(`/([^/]+)/([^/]+).git`)

const glRawManifestURL = "https://%s/%s/%s/raw/%s/%s"

type gitFetcher struct {
	fs          afero.Fs
	manFilename string
}

func newGitFetcher(fs afero.Fs, manFilename string) *gitFetcher {
	return &gitFetcher{fs: fs, manFilename: manFilename}
}

var manifestClient = &http.Client{
	Timeout: 60 * time.Second,
}

func isGithub(src *url.URL) bool {
	return src.Host == "github.com"
}

func isGitlab(src *url.URL) bool {
	return src.Host == "framagit.org" || strings.Contains(src.Host, "gitlab")
}

func (g *gitFetcher) FetchManifest(src *url.URL) (io.ReadCloser, error) {
	var err error

	var u string
	if isGithub(src) {
		u, err = resolveGithubURL(src, g.manFilename)
	} else if isGitlab(src) {
		u, err = resolveGitlabURL(src, g.manFilename)
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

func (g *gitFetcher) Fetch(src *url.URL, baseDir string) error {
	log.Debugf("[git] Fetch %s", src.String())
	fs := g.fs

	gitDir := path.Join(baseDir, ".git")
	exists, err := afero.DirExists(fs, gitDir)
	if err != nil {
		return err
	}
	if exists {
		return g.pull(baseDir, gitDir, src)
	}
	if err = fs.Mkdir(gitDir, 0755); err != nil {
		return err
	}
	return g.clone(baseDir, gitDir, src)
}

func getGitBranch(src *url.URL) string {
	if src.Fragment != "" {
		return "refs/heads/" + src.Fragment
	}
	return "HEAD"
}

func getWebBranch(src *url.URL) string {
	if src.Fragment != "" {
		return src.Fragment
	}
	return "HEAD"
}

// clone creates a new bare git repository and install all the files of the
// last commit in the application tree.
func (g *gitFetcher) clone(baseDir, gitDir string, src *url.URL) error {
	fs := g.fs

	storage, err := gitStorage.NewStorage(newGFS(fs, gitDir))
	if err != nil {
		return err
	}

	branch := getGitBranch(src)
	log.Debugf("[git] Clone %s %s", src.String(), branch)

	// XXX Gitlab doesn't support the git protocol
	if isGitlab(src) {
		src.Scheme = "https"
		src.Fragment = ""
	}

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
func (g *gitFetcher) pull(baseDir, gitDir string, src *url.URL) error {
	fs := g.fs

	storage, err := gitStorage.NewStorage(newGFS(fs, gitDir))
	if err != nil {
		return err
	}

	rep, err := git.Open(storage, nil)
	if err != nil {
		return err
	}

	branch := getGitBranch(src)
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

	err = afero.Walk(fs, baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == baseDir {
			return nil
		}
		if filepath.Base(path) == ".git" {
			return filepath.SkipDir
		}

		if info.IsDir() {
			err = fs.RemoveAll(path)
		} else {
			err = fs.Remove(path)
		}
		if err != nil {
			return err
		}

		if info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}

	return g.copyFiles(baseDir, rep)
}

func (g *gitFetcher) copyFiles(baseDir string, rep *git.Repository) error {
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
		abs := path.Join(baseDir, f.Name)
		dir := path.Dir(abs)

		if err := fs.MkdirAll(dir, 0755); err != nil {
			return err
		}

		file, err := fs.Create(abs)
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
	branch := getWebBranch(src)

	u := fmt.Sprintf(ghRawManifestURL, user, project, branch, filename)
	return u, nil
}

func resolveGitlabURL(src *url.URL, filename string) (string, error) {
	match := glURLRegex.FindStringSubmatch(src.Path)
	if len(match) != 3 {
		return "", &url.Error{
			Op:  "parsepath",
			URL: src.String(),
			Err: errors.New("Could not parse url git path"),
		}
	}

	user, project := match[1], match[2]
	branch := getWebBranch(src)

	u := fmt.Sprintf(glRawManifestURL, src.Host, user, project, branch, filename)
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
	fs   afero.Fs
	base string
}

type gfile struct {
	f      afero.File
	name   string
	closed bool
}

func newGFile(f afero.File, name string) *gfile {
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

func newGFS(fs afero.Fs, base string) *gfs {
	return &gfs{
		fs:   fs,
		base: path.Clean(base),
	}
}

func (fs *gfs) OpenFile(name string, flag int, perm os.FileMode) (gitFS.File, error) {
	fullpath := path.Join(fs.base, name)
	if flag&os.O_CREATE != 0 {
		if err := fs.createDir(fullpath); err != nil {
			return nil, err
		}
	}
	file, err := fs.fs.OpenFile(fullpath, flag, perm)
	if err != nil {
		return nil, err
	}
	return newGFile(file, fullpath[len(fs.base):]), nil
}

func (fs *gfs) Create(name string) (gitFS.File, error) {
	return fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

func (fs *gfs) Open(name string) (gitFS.File, error) {
	fullpath := fs.Join(fs.base, name)
	f, err := fs.fs.OpenFile(fullpath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return newGFile(f, fullpath[len(fs.base):]), nil
}

func (fs *gfs) Remove(name string) error {
	return fs.fs.Remove(fs.Join(fs.base, name))
}

func (fs *gfs) Stat(name string) (gitFS.FileInfo, error) {
	return fs.fs.Stat(fs.Join(fs.base, name))
}

func (fs *gfs) ReadDir(name string) ([]gitFS.FileInfo, error) {
	is, err := afero.ReadDir(fs.fs, fs.Join(fs.base, name))
	if err != nil {
		return nil, err
	}
	infos := make([]gitFS.FileInfo, len(is))
	for i := range is {
		infos[i] = is[i]
	}
	return infos, nil
}

func (fs *gfs) MkdirAll(path string, perm os.FileMode) error {
	return fs.fs.MkdirAll(fs.Join(fs.base, path), perm)
}

func (fs *gfs) TempFile(dirname, prefix string) (gitFS.File, error) {
	fullpath := fs.Join(fs.base, dirname)
	if err := fs.createDir(fullpath + "/"); err != nil {
		return nil, err
	}
	file, err := afero.TempFile(fs.fs, fullpath, prefix)
	if err != nil {
		return nil, err
	}
	filename := path.Join(fullpath[len(fs.base):], path.Base(file.Name()))
	return newGFile(file, filename), nil
}

func (fs *gfs) Rename(from, to string) error {
	from = fs.Join(fs.base, from)
	to = fs.Join(fs.base, to)
	if err := fs.createDir(to); err != nil {
		return err
	}
	return fs.fs.Rename(from, to)
}

func (fs *gfs) Join(elem ...string) string {
	return path.Join(elem...)
}

func (fs *gfs) Dir(name string) gitFS.Filesystem {
	return newGFS(fs.fs, fs.Join(fs.base, name))
}

func (fs *gfs) createDir(fullpath string) error {
	dir := filepath.Dir(fullpath)
	if dir != "." {
		if err := fs.fs.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func (fs *gfs) Base() string {
	return fs.base
}

var (
	_ Fetcher          = &gitFetcher{}
	_ gitFS.Filesystem = &gfs{}
	_ gitFS.File       = &gfile{}
)
