package app_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/cozy/afero"
	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
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

func serveGitRep() {
	dir, err := ioutil.TempDir("", "cozy-app")
	if err != nil {
		panic(err)
	}
	localGitDir = dir
	args := `
echo '` + manifestWebapp() + `' > ` + app.WebappManifestName + ` && \
echo '` + manifestKonnector() + `' > ` + app.KonnectorManifestName + ` && \
git init . && \
git add . && \
git commit -m 'Initial commit' && \
git checkout -b branch && \
echo 'branch' > branch && \
git add . && \
git commit -m 'Create a branch' && \
git checkout -`
	cmd := exec.Command("sh", "-c", args)
	cmd.Dir = localGitDir
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	// "git daemon --reuseaddr --base-path=./ --export-all ./.git"
	localGitCmd = exec.Command("git", "daemon", "--reuseaddr", "--base-path=./", "--export-all", "./.git")
	localGitCmd.Dir = localGitDir
	if out, err := localGitCmd.CombinedOutput(); err != nil {
		fmt.Println(string(out))
		os.Exit(1)
	}
}

func doUpgrade(major int) {
	localVersion = fmt.Sprintf("%d.0.0", major)
	args := `
echo '` + manifestWebapp() + `' > ` + app.WebappManifestName + ` && \
echo '` + manifestKonnector() + `' > ` + app.KonnectorManifestName + ` && \
git commit -am "Upgrade commit" && \
git checkout branch && \
git rebase master && \
git checkout master`
	cmd := exec.Command("sh", "-c", args)
	cmd.Dir = localGitDir
	if out, err := cmd.Output(); err != nil {
		fmt.Println(string(out), err)
	} else {
		fmt.Println("did upgrade", localVersion)
	}
}

var db prefixer.Prefixer
var fs appfs.Copier
var baseFS afero.Fs

func TestMain(m *testing.M) {
	config.UseTestFile()

	check, err := checkup.HTTPChecker{URL: config.CouchURL().String()}.Check()
	if err != nil || check.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	_, err = stack.Start()
	if err != nil {
		fmt.Println("Error while starting job system", err)
		os.Exit(1)
	}

	app.ManifestClient = &http.Client{
		Transport: &transport{},
	}

	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, manGen())
	}))

	db = prefixer.NewPrefixer("", "app-test")

	err = couchdb.ResetDB(db, consts.Apps)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.ResetDB(db, consts.Konnectors)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.ResetDB(db, consts.Files)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var tmpDir string
	osFS := afero.NewOsFs()
	tmpDir, err = afero.TempDir(osFS, "", "cozy-installer-test")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer func() { _ = osFS.RemoveAll(tmpDir) }()

	baseFS = afero.NewBasePathFs(osFS, tmpDir)
	fs = appfs.NewAferoCopier(baseFS)

	go serveGitRep()

	time.Sleep(100 * time.Millisecond)

	err = couchdb.ResetDB(db, consts.Permissions)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndexes(db, couchdb.IndexesByDoctype(consts.Files))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndexes(db, couchdb.IndexesByDoctype(consts.Permissions))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()

	_ = couchdb.DeleteDB(db, consts.Apps)
	_ = couchdb.DeleteDB(db, consts.Konnectors)
	_ = couchdb.DeleteDB(db, consts.Files)
	_ = couchdb.DeleteDB(db, consts.Permissions)
	ts.Close()

	_ = localGitCmd.Process.Signal(os.Interrupt)

	os.Exit(res)
}
