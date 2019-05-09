package exec

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

var inst *instance.Instance

func TestUnknownDomain(t *testing.T) {
	msg, err := job.NewMessage(map[string]interface{}{
		"konnector": "unknownapp",
	})
	assert.NoError(t, err)
	db := prefixer.NewPrefixer("instance.does.not.exist", "instance.does.not.exist")
	j := job.NewJob(db, &job.JobRequest{
		Message:    msg,
		WorkerType: "konnector",
	})
	ctx := job.NewWorkerContext("id", j, nil).
		WithCookie(&konnectorWorker{})
	err = worker(ctx)
	assert.Error(t, err)
	assert.Equal(t, "Instance not found", err.Error())
}

func TestUnknownApp(t *testing.T) {
	msg, err := job.NewMessage(map[string]interface{}{
		"konnector": "unknownapp",
	})
	assert.NoError(t, err)
	j := job.NewJob(inst, &job.JobRequest{
		Message:    msg,
		WorkerType: "konnector",
	})
	ctx := job.NewWorkerContext("id", j, inst).
		WithCookie(&konnectorWorker{})
	err = worker(ctx)
	assert.Error(t, err)
	assert.Equal(t, "Application is not installed", err.Error())
}

func TestBadFileExec(t *testing.T) {
	folderToSave := "7890"

	installer, err := app.NewInstaller(inst, inst.AppsCopier(consts.KonnectorType),
		&app.InstallerOptions{
			Operation: app.Install,
			Type:      consts.KonnectorType,
			Slug:      "my-konnector-1",
			SourceURL: "git://github.com/konnectors/cozy-konnector-trainline.git",
		},
	)
	if !assert.NoError(t, err) {
		return
	}
	_, err = installer.RunSync()
	if !assert.NoError(t, err) {
		return
	}

	msg, err := job.NewMessage(map[string]interface{}{
		"konnector":      "my-konnector-1",
		"folder_to_save": folderToSave,
	})
	assert.NoError(t, err)

	j := job.NewJob(inst, &job.JobRequest{
		Message:    msg,
		WorkerType: "konnector",
	})

	config.GetConfig().Konnectors.Cmd = ""
	ctx := job.NewWorkerContext("id", j, inst).
		WithCookie(&konnectorWorker{})
	err = worker(ctx)
	assert.Error(t, err)
	assert.Equal(t, "fork/exec : no such file or directory", err.Error())

	config.GetConfig().Konnectors.Cmd = "echo"
	err = worker(ctx)
	assert.NoError(t, err)
}

func TestSuccess(t *testing.T) {
	script := `#!/bin/bash

echo "{\"type\": \"toto\", \"message\": \"COZY_URL=${COZY_URL} ${COZY_CREDENTIALS}\"}"
echo "bad json"
echo "{\"type\": \"manifest\", \"message\": \"$(ls ${1}/manifest.konnector)\" }"
>&2 echo "log error"
`
	osFs := afero.NewOsFs()
	tmpScript := fmt.Sprintf("/tmp/test-konn-%d.sh", os.Getpid())
	defer func() { _ = osFs.RemoveAll(tmpScript) }()

	err := afero.WriteFile(osFs, tmpScript, []byte(script), 0)
	if !assert.NoError(t, err) {
		return
	}

	err = osFs.Chmod(tmpScript, 0777)
	if !assert.NoError(t, err) {
		return
	}

	installer, err := app.NewInstaller(inst, inst.AppsCopier(consts.KonnectorType),
		&app.InstallerOptions{
			Operation: app.Install,
			Type:      consts.KonnectorType,
			Slug:      "my-konnector-1",
			SourceURL: "git://github.com/konnectors/cozy-konnector-trainline.git",
		},
	)
	if err != app.ErrAlreadyExists {
		if !assert.NoError(t, err) {
			return
		}
		_, err = installer.RunSync()
		if !assert.NoError(t, err) {
			return
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		evCh := realtime.GetHub().Subscriber(inst)
		assert.NoError(t, evCh.Subscribe(consts.JobEvents))
		wg.Done()
		ch := evCh.Channel
		ev1 := <-ch
		ev2 := <-ch
		err = evCh.Close()
		assert.NoError(t, err)
		doc1 := ev1.Doc.(couchdb.JSONDoc)
		doc2 := ev2.Doc.(couchdb.JSONDoc)

		assert.Equal(t, inst.Domain, ev1.Domain)
		assert.Equal(t, inst.Domain, ev2.Domain)

		assert.Equal(t, "toto", doc1.M["type"])
		assert.Equal(t, "manifest", doc2.M["type"])

		msg2 := doc2.M["message"].(string)
		assert.True(t, strings.HasPrefix(msg2, os.TempDir()))
		assert.True(t, strings.HasSuffix(msg2, "/manifest.konnector"))

		msg1 := doc1.M["message"].(string)
		cozyURL := "COZY_URL=" + inst.PageURL("/", nil) + " "
		assert.True(t, strings.HasPrefix(msg1, cozyURL))
		token := msg1[len(cozyURL):]
		var claims permission.Claims
		err = crypto.ParseJWT(token, func(t *jwt.Token) (interface{}, error) {
			return inst.PickKey(t.Claims.(*permission.Claims).Audience)
		}, &claims)
		assert.NoError(t, err)
		assert.Equal(t, consts.KonnectorAudience, claims.Audience)
		wg.Done()
	}()

	wg.Wait()
	wg.Add(1)
	msg, err := job.NewMessage(map[string]interface{}{
		"konnector": "my-konnector-1",
	})
	assert.NoError(t, err)

	j := job.NewJob(inst, &job.JobRequest{
		Message:    msg,
		WorkerType: "konnector",
	})

	config.GetConfig().Konnectors.Cmd = tmpScript
	ctx := job.NewWorkerContext("id", j, inst).
		WithCookie(&konnectorWorker{})
	err = worker(ctx)
	assert.NoError(t, err)

	wg.Wait()
}

func TestSecretFromAccountType(t *testing.T) {
	script := `#!/bin/bash

SECRET=$(echo "$COZY_PARAMETERS" | sed -e 's/.*secret"://' -e 's/[},].*//')
echo "{\"type\": \"params\", \"message\": ${SECRET} }"
`
	osFs := afero.NewOsFs()
	tmpScript := fmt.Sprintf("/tmp/test-konn-%d.sh", os.Getpid())
	defer func() { _ = osFs.RemoveAll(tmpScript) }()

	err := afero.WriteFile(osFs, tmpScript, []byte(script), 0)
	if !assert.NoError(t, err) {
		return
	}

	err = osFs.Chmod(tmpScript, 0777)
	if !assert.NoError(t, err) {
		return
	}

	at := &account.AccountType{
		GrantMode: "secret",
		Slug:      "my-konnector-1",
		Secret:    "s3cr3t",
	}
	err = couchdb.CreateDoc(couchdb.GlobalSecretsDB, at)
	assert.NoError(t, err)
	defer func() {
		// Clean the account types
		ats, _ := account.FindAccountTypesBySlug("my-konnector-1")
		for _, at = range ats {
			_ = couchdb.DeleteDoc(couchdb.GlobalSecretsDB, at)
		}
	}()

	installer, err := app.NewInstaller(inst, inst.AppsCopier(consts.KonnectorType),
		&app.InstallerOptions{
			Operation: app.Install,
			Type:      consts.KonnectorType,
			Slug:      "my-konnector-1",
			SourceURL: "git://github.com/konnectors/cozy-konnector-trainline.git",
		},
	)
	if err != app.ErrAlreadyExists {
		if !assert.NoError(t, err) {
			return
		}
		_, err = installer.RunSync()
		if !assert.NoError(t, err) {
			return
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		evCh := realtime.GetHub().Subscriber(inst)
		assert.NoError(t, evCh.Subscribe(consts.JobEvents))
		wg.Done()
		ch := evCh.Channel
		ev1 := <-ch
		err = evCh.Close()
		assert.NoError(t, err)
		doc1 := ev1.Doc.(couchdb.JSONDoc)

		assert.Equal(t, inst.Domain, ev1.Domain)
		assert.Equal(t, "params", doc1.M["type"])
		msg1 := doc1.M["message"]
		assert.Equal(t, "s3cr3t", msg1)
		wg.Done()
	}()

	wg.Wait()
	wg.Add(1)
	msg, err := job.NewMessage(map[string]interface{}{
		"konnector": "my-konnector-1",
	})
	assert.NoError(t, err)

	j := job.NewJob(inst, &job.JobRequest{
		Message:    msg,
		WorkerType: "konnector",
	})

	config.GetConfig().Konnectors.Cmd = tmpScript
	ctx := job.NewWorkerContext("id", j, inst).
		WithCookie(&konnectorWorker{})
	err = worker(ctx)
	assert.NoError(t, err)

	wg.Wait()
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	setup := testutils.NewSetup(m, "konnector_test")
	inst = setup.GetTestInstance()
	os.Exit(setup.Run())
}
