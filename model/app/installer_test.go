package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"testing"

	"github.com/cozy/cozy-stack/model/app"
)

var localGitCmd *exec.Cmd
var localGitDir string
var localVersion string
var localServices string
var ts *httptest.Server

var manGen func() string
var manName string

type transport struct{}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := new(http.Request)
	*req2 = *req
	req2.URL, _ = url.Parse(ts.URL)
	return http.DefaultTransport.RoundTrip(req2)
}

func manifestWebapp() string {
	if localServices == "" {
		localServices = "{}"
	}
	return `{
  "description": "A mini app to test cozy-stack-v2",
  "developer": {
    "name": "Bruno",
    "url": "cozy.io"
  },
  "license": "MIT",
  "name": "mini-app",
  "permissions": {},
  "slug": "mini",
  "type": "webapp",
  "version": "` + localVersion + `",
  "services": ` + localServices + `
}`
}

func manifestKonnector() string {
	return `{
  "description": "A mini konnector to test cozy-stack-v2",
  "type": "node",
  "developer": {
    "name": "Bruno",
    "url": "cozy.io"
  },
  "license": "MIT",
  "name": "mini-app",
  "permissions": {},
  "slug": "mini",
  "type": "konnector",
  "version": "` + localVersion + `"
}`
}

func serveGitRep(dir string) {
	localGitDir = dir
	args := `
echo '` + manifestWebapp() + `' > ` + app.WebappManifestName + ` && \
echo '` + manifestKonnector() + `' > ` + app.KonnectorManifestName + ` && \
git init . && \
git config user.name "cozy" && \
git config user.email "cozy@cloud.fr" && \
git add . && \
git commit -m 'Initial commit' && \
git checkout -b branch && \
echo 'branch' > branch && \
git add . && \
git commit -m 'Create a branch' && \
git checkout -`
	cmd := exec.Command("bash", "-c", args)
	cmd.Dir = localGitDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Can't call t.Fatalf inside an another gorouting
		panic(fmt.Sprintf("failed to setup the git repo (output: %q): %s", out, err))
	}

	// "git daemon --reuseaddr --base-path=./ --export-all ./.git"
	localGitCmd = exec.Command("git", "daemon", "--reuseaddr", "--base-path=./", "--export-all", "./.git")
	localGitCmd.Dir = localGitDir
	if out, err := localGitCmd.CombinedOutput(); err != nil {
		// Can't call t.Fatalf inside an another gorouting
		panic(fmt.Sprintf("failed start the git server (output: %q): %s", out, err))
	}
}

func doUpgrade(t *testing.T, major int) {
	localVersion = fmt.Sprintf("%d.0.0", major)
	args := `
echo '` + manifestWebapp() + `' > ` + app.WebappManifestName + ` && \
echo '` + manifestKonnector() + `' > ` + app.KonnectorManifestName + ` && \
git commit -am "Upgrade commit" && \
git checkout branch && \
git rebase master && \
git checkout master`
	cmd := exec.Command("bash", "-c", args)
	cmd.Dir = localGitDir
	if out, err := cmd.Output(); err != nil {
		t.Log(string(out), err)
	} else {
		t.Log("did upgrade", localVersion)
	}
}
