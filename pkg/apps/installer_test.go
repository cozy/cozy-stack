package apps

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

var localGitCmd *exec.Cmd
var localGitDir string
var localVersion string
var ts *httptest.Server

var installerType AppType
var manifestName string

type transport struct{}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := new(http.Request)
	*req2 = *req
	req2.URL, _ = url.Parse(ts.URL)
	return http.DefaultTransport.RoundTrip(req2)
}

func manifestWebapp() string {
	return strings.Replace(`{
  "description": "A mini app to test cozy-stack-v2",
  "developer": {
    "name": "Bruno",
    "url": "cozy.io"
  },
  "license": "MIT",
  "name": "mini-app",
  "permissions": {},
  "slug": "mini",
  "version": "`+localVersion+`"
}`, "\n", "", -1)
}

func manifestKonnector() string {
	return strings.Replace(`{
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
  "version": "`+localVersion+`"
}`, "\n", "", -1)
}

func serveGitRep(manName string, manGen func() string) {
	dir, err := ioutil.TempDir("", "cozy-app")
	if err != nil {
		panic(err)
	}
	localGitDir = dir
	args := `
echo '` + manGen() + `' > ` + manName + ` && \
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
	}
}

func doUpgrade(major int) {
	localVersion = fmt.Sprintf("%d.0.0", major)
	args := `
echo '` + manifestWebapp() + `' > ` + manifestName + ` && \
git commit -am "Upgrade commit" && \
git checkout - && \
echo '` + manifestWebapp() + `' > ` + manifestName + ` && \
git commit -am "Upgrade commit" && \
git checkout -`
	cmd := exec.Command("sh", "-c", args)
	cmd.Dir = localGitDir
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

var db couchdb.Database
var fs afero.Fs

func TestInstallBadSlug(t *testing.T) {
	_, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		SourceURL: "git://foo.bar",
	})
	if assert.Error(t, err) {
		assert.Equal(t, ErrInvalidSlugName, err)
	}

	_, err = NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "coucou/",
		SourceURL: "git://foo.bar",
	})
	if assert.Error(t, err) {
		assert.Equal(t, ErrInvalidSlugName, err)
	}
}

func TestInstallBadAppsSource(t *testing.T) {
	_, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "app3",
		SourceURL: "foo://bar.baz",
	})
	if assert.Error(t, err) {
		assert.Equal(t, ErrNotSupportedSource, err)
	}

	_, err = NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "app4",
		SourceURL: "git://bar  .baz",
	})
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "invalid character")
	}

	_, err = NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "app5",
		SourceURL: "",
	})
	if assert.Error(t, err) {
		assert.Equal(t, ErrMissingSource, err)
	}
}

func TestInstallSuccessful(t *testing.T) {
	inst, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "local-cozy-mini",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Install()

	var state State
	for {
		man, done, err2 := inst.Poll()
		if !assert.NoError(t, err2) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, Installing, man.State()) {
				return
			}
		} else if state == Installing {
			if !assert.EqualValues(t, Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}

	ok, err := afero.Exists(fs, "/local-cozy-mini/"+manifestName)
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = afero.FileContainsBytes(fs, "/local-cozy-mini/"+manifestName, []byte("1.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")

	inst2, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "local-cozy-mini",
		SourceURL: "git://localhost/",
	})
	assert.Nil(t, inst2)
	assert.Equal(t, ErrAlreadyExists, err)
}

func TestUpgradeNotExist(t *testing.T) {
	inst, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Update,
		Type:      installerType,
		Slug:      "cozy-app-not-exist",
	})
	assert.Nil(t, inst)
	assert.Equal(t, ErrNotFound, err)

	inst, err = NewInstaller(db, fs, &InstallerOptions{
		Operation: Delete,
		Type:      installerType,
		Slug:      "cozy-app-not-exist",
	})
	assert.Nil(t, inst)
	assert.Equal(t, ErrNotFound, err)
}

func TestInstallWithUpgrade(t *testing.T) {
	inst, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "cozy-app-b",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Install()

	for {
		var done bool
		_, done, err = inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if done {
			break
		}
	}

	ok, err := afero.Exists(fs, "/local-cozy-mini/"+manifestName)
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = afero.FileContainsBytes(fs, "/local-cozy-mini/"+manifestName, []byte("1.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")

	doUpgrade(2)

	inst, err = NewInstaller(db, fs, &InstallerOptions{
		Operation: Update,
		Type:      installerType,
		Slug:      "cozy-app-b",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Update()

	var state State
	for {
		man, done, err2 := inst.Poll()
		if !assert.NoError(t, err2) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, Upgrading, man.State()) {
				return
			}
		} else if state == Upgrading {
			if !assert.EqualValues(t, Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}

	ok, err = afero.Exists(fs, "/cozy-app-b/"+manifestName)
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = afero.FileContainsBytes(fs, "/cozy-app-b/"+manifestName, []byte("2.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
}

func TestInstallAndUpgradeWithBranch(t *testing.T) {
	doUpgrade(3)

	inst, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "local-cozy-mini-branch",
		SourceURL: "git://localhost/#branch",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Install()

	var state State
	for {
		man, done, err2 := inst.Poll()
		if !assert.NoError(t, err2) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, Installing, man.State()) {
				return
			}
		} else if state == Installing {
			if !assert.EqualValues(t, Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}

	ok, err := afero.Exists(fs, "/local-cozy-mini-branch/"+manifestName)
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = afero.FileContainsBytes(fs, "/local-cozy-mini-branch/"+manifestName, []byte("3.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	ok, err = afero.Exists(fs, "/local-cozy-mini-branch/branch")
	assert.NoError(t, err)
	assert.True(t, ok, "The good branch was checked out")

	doUpgrade(4)

	inst, err = NewInstaller(db, fs, &InstallerOptions{
		Operation: Update,
		Type:      installerType,
		Slug:      "local-cozy-mini-branch",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Update()

	state = ""
	for {
		man, done, err2 := inst.Poll()
		if !assert.NoError(t, err2) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, Upgrading, man.State()) {
				return
			}
		} else if state == Upgrading {
			if !assert.EqualValues(t, Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}

	ok, err = afero.Exists(fs, "/local-cozy-mini-branch/"+manifestName)
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest is present")
	ok, err = afero.FileContainsBytes(fs, "/local-cozy-mini-branch/"+manifestName, []byte("4.0.0"))
	assert.NoError(t, err)
	assert.True(t, ok, "The manifest has the right version")
	ok, err = afero.Exists(fs, "/local-cozy-mini-branch/branch")
	assert.NoError(t, err)
	assert.True(t, ok, "The good branch was checked out")
}

func TestInstallFromGithub(t *testing.T) {
	inst, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "github-cozy-mini",
		SourceURL: "git://github.com/nono/cozy-mini.git",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Install()

	var state State
	for {
		man, done, err := inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, Installing, man.State()) {
				return
			}
		} else if state == Installing {
			if !assert.EqualValues(t, Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}
}

func TestInstallFromGitlab(t *testing.T) {
	inst, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "gitlab-cozy-mini",
		SourceURL: "git://framagit.org/nono/cozy-mini.git",
	})
	if !assert.NoError(t, err) {
		return
	}

	go inst.Install()

	var state State
	for {
		man, done, err := inst.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if state == "" {
			if !assert.EqualValues(t, Installing, man.State()) {
				return
			}
		} else if state == Installing {
			if !assert.EqualValues(t, Ready, man.State()) {
				return
			}
			if !assert.True(t, done) {
				return
			}
			break
		} else {
			t.Fatalf("invalid state")
			return
		}
		state = man.State()
	}
}

func TestUninstall(t *testing.T) {
	inst1, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Install,
		Type:      installerType,
		Slug:      "github-cozy-delete",
		SourceURL: "git://localhost/",
	})
	if !assert.NoError(t, err) {
		return
	}
	go inst1.Install()
	for {
		var done bool
		_, done, err = inst1.Poll()
		if !assert.NoError(t, err) {
			return
		}
		if done {
			break
		}
	}
	inst2, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Delete,
		Type:      installerType,
		Slug:      "github-cozy-delete",
	})
	if !assert.NoError(t, err) {
		return
	}
	_, err = inst2.Delete()
	if !assert.NoError(t, err) {
		return
	}
	inst3, err := NewInstaller(db, fs, &InstallerOptions{
		Operation: Delete,
		Type:      installerType,
		Slug:      "github-cozy-delete",
	})
	assert.Nil(t, inst3)
	assert.Equal(t, ErrNotFound, err)
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	check, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || check.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	manifestClient = &http.Client{
		Transport: &transport{},
	}

	res1 := RunTest(
		m,
		Webapp,
		consts.Apps,
		vfs.WebappsDirName,
		WebappManifestName,
		manifestWebapp,
	)

	res2 := RunTest(
		m,
		Konnector,
		consts.Konnectors,
		vfs.KonnectorsDirName,
		KonnectorManifestName,
		manifestKonnector,
	)

	os.Exit(res1 + res2)
}

func RunTest(m *testing.M, appType AppType, dbName, instDir, manName string, manGen func() string) int {
	localVersion = "1.0.0"
	manifestName = manName
	installerType = appType

	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, manGen())
	}))

	db = couchdb.SimpleDatabasePrefix("apps-test")

	err := couchdb.ResetDB(db, dbName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.ResetDB(db, consts.Files)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fs = afero.NewMemMapFs()

	go serveGitRep(manName, manGen)

	time.Sleep(100 * time.Millisecond)

	err = couchdb.ResetDB(db, consts.Permissions)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndexes(db, consts.IndexesByDoctype(consts.Files))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndexes(db, consts.IndexesByDoctype(consts.Permissions))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	res := m.Run()

	couchdb.DeleteDB(db, dbName)
	couchdb.DeleteDB(db, consts.Files)
	couchdb.DeleteDB(db, consts.Permissions)
	ts.Close()

	localGitCmd.Process.Signal(os.Interrupt)
	return res
}
