package apps

import (
	"archive/tar"
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
	manFilename string
	createTar   bool
}

func newGitFetcher(manFilename string, createTar bool) *gitFetcher {
	return &gitFetcher{
		manFilename: manFilename,
		createTar:   createTar,
	}
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

func (g *gitFetcher) Fetch(src *url.URL, fs Copier, slug, version string) error {
	log.Debugf("[git] Fetch %s", src.String())

	localFs := afero.NewOsFs()
	gitDir, err := afero.TempDir(localFs, "", "cozy-app-"+slug)
	if err != nil {
		return err
	}
	defer localFs.RemoveAll(gitDir)

	localFs = afero.NewBasePathFs(localFs, gitDir)
	storage, err := gitStorage.NewStorage(newGFS(localFs))
	if err != nil {
		return err
	}

	branch := getGitBranch(src)
	log.Debugf("[git] Clone %s %s in %s", src.String(), branch, gitDir)

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

	return g.copyFiles(fs, localFs, rep, slug, version)
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

func (g *gitFetcher) copyFiles(fs Copier, localFs afero.Fs, rep *git.Repository, slug, version string) error {
	ref, err := rep.Head()
	if err != nil {
		return err
	}

	version = version + "_" + ref.Hash().String()

	exists, err := fs.Start(slug, version)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	commit, err := rep.CommitObject(ref.Hash())
	if err != nil {
		return err
	}
	files, err := commit.Files()
	if err != nil {
		return err
	}

	if !g.createTar {
		err = files.ForEach(func(f *gitObject.File) error {
			r, err := f.Reader()
			if err != nil {
				return err
			}
			defer r.Close()
			return fs.Copy(slug, version, f.Name, r)
		})
		if err != nil {
			return err
		}
	} else {
		tmp, err := afero.TempFile(localFs, "", "")
		if err != nil {
			return err
		}
		defer localFs.Remove(tmp.Name())

		tw := tar.NewWriter(tmp)

		err = files.ForEach(func(f *gitObject.File) error {
			r, err := f.Reader()
			if err != nil {
				return err
			}
			defer r.Close()
			hdr := &tar.Header{
				Name: f.Name,
				Mode: 0600,
				Size: f.Size,
			}
			if err = tw.WriteHeader(hdr); err != nil {
				return err
			}
			_, err = io.Copy(tw, r)
			return err
		})
		if err != nil {
			return err
		}

		if err = tw.Flush(); err != nil {
			return err
		}

		tmp.Seek(0, 0)
		if err = fs.Copy(slug, version, "", tmp); err != nil {
			return err
		}
	}

	return fs.Close()
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
	fs afero.Fs
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

func newGFS(fs afero.Fs) *gfs {
	return &gfs{fs: fs}
}

func (fs *gfs) OpenFile(name string, flag int, perm os.FileMode) (gitFS.File, error) {
	if flag&os.O_CREATE != 0 {
		if err := fs.createDir(name); err != nil {
			return nil, err
		}
	}
	file, err := fs.fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return newGFile(file, name), nil
}

func (fs *gfs) Create(name string) (gitFS.File, error) {
	return fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

func (fs *gfs) Open(name string) (gitFS.File, error) {
	f, err := fs.fs.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return newGFile(f, name), nil
}

func (fs *gfs) Remove(name string) error {
	return fs.fs.Remove(name)
}

func (fs *gfs) Stat(name string) (gitFS.FileInfo, error) {
	return fs.fs.Stat(name)
}

func (fs *gfs) ReadDir(name string) ([]gitFS.FileInfo, error) {
	is, err := afero.ReadDir(fs.fs, name)
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
	return fs.fs.MkdirAll(path, perm)
}

func (fs *gfs) TempFile(dirname, prefix string) (gitFS.File, error) {
	if err := fs.createDir(dirname + "/"); err != nil {
		return nil, err
	}
	file, err := afero.TempFile(fs.fs, dirname, prefix)
	if err != nil {
		return nil, err
	}
	return newGFile(file, file.Name()), nil
}

func (fs *gfs) Rename(from, to string) error {
	if err := fs.createDir(to); err != nil {
		return err
	}
	return fs.fs.Rename(from, to)
}

func (fs *gfs) Join(elem ...string) string {
	return path.Join(elem...)
}

func (fs *gfs) Dir(name string) gitFS.Filesystem {
	return newGFS(afero.NewBasePathFs(fs.fs, name))
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
	return "/"
}

var (
	_ Fetcher          = &gitFetcher{}
	_ gitFS.Filesystem = &gfs{}
	_ gitFS.File       = &gfile{}
)
