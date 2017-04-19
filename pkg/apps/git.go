package apps

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/afero"
	gitOsFS "gopkg.in/src-d/go-billy.v2/osfs"
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
}

func newGitFetcher(appType AppType) *gitFetcher {
	var manFilename string
	switch appType {
	case Webapp:
		manFilename = WebappManifestName
	case Konnector:
		manFilename = KonnectorManifestName
	}
	return &gitFetcher{
		manFilename: manFilename,
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

func (g *gitFetcher) Fetch(src *url.URL, fs Copier, man Manifest) error {
	log.Debugf("[git] Fetch %s", src.String())

	osFs := afero.NewOsFs()
	gitDir, err := afero.TempDir(osFs, "", "cozy-app-"+man.Slug())
	if err != nil {
		return err
	}
	defer osFs.RemoveAll(gitDir)

	storage, err := gitStorage.NewStorage(gitOsFS.New(gitDir))
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

	ref, err := rep.Head()
	if err != nil {
		return err
	}

	slug := man.Slug()
	version := man.Version() + "-" + ref.Hash().String()

	// The git fetcher needs to update the actual version of the application to
	// reflect the git version of the repository.
	man.SetVersion(version)

	// If the application folder already exists, we can bail early.
	exists, err := fs.Start(slug, version)
	if err != nil {
		return err
	}
	defer func() {
		if errc := fs.Close(); errc != nil {
			err = errc
		}
	}()
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

	return files.ForEach(func(f *gitObject.File) error {
		var r io.ReadCloser
		r, err = f.Reader()
		if err != nil {
			return err
		}
		defer r.Close()
		return fs.Copy(&fileInfo{
			name: f.Name,
			size: f.Size,
			mode: os.FileMode(f.Mode),
		}, r)
	})
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

var (
	_ Fetcher = &gitFetcher{}
)
