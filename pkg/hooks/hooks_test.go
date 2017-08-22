package hooks

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestExecuteSuccess(t *testing.T) {
	tmp := fmt.Sprintf("%s/foo-%d", os.TempDir(), time.Now().Unix())
	args := []string{tmp, "bar"}
	defer os.Remove(tmp)
	executed := false
	fn := func() error { executed = true; return nil }
	err := Execute("success", args, fn)
	assert.NoError(t, err)
	assert.True(t, executed)
	content, err := ioutil.ReadFile(tmp)
	assert.NoError(t, err)
	assert.Equal(t, "post-bar\n", string(content))
}

func TestExecutePreFails(t *testing.T) {
	executed := false
	fn := func() error { executed = true; return nil }
	err := Execute("failure", []string{}, fn)
	assert.Equal(t, ErrHookFailed, err)
	assert.False(t, executed)
}

func TestExecuteFunctionFails(t *testing.T) {
	tmp := fmt.Sprintf("%s/foo-%d", os.TempDir(), time.Now().Unix())
	args := []string{tmp, "bar"}
	defer os.Remove(tmp)
	executed := false
	e := errors.New("fn fails")
	fn := func() error { executed = true; return e }
	err := Execute("success", args, fn)
	assert.Equal(t, e, err)
	assert.True(t, executed)
	content, err := ioutil.ReadFile(tmp)
	assert.NoError(t, err)
	assert.Equal(t, "pre-bar\n", string(content))
}

func TestRunHooks(t *testing.T) {
	tmp := fmt.Sprintf("%s/foo-%d", os.TempDir(), time.Now().Unix())
	args := []string{tmp, "bar"}
	defer os.Remove(tmp)
	err := runHook("pre", "success", args)
	assert.NoError(t, err)
	content, err := ioutil.ReadFile(tmp)
	assert.NoError(t, err)
	assert.Equal(t, "pre-bar\n", string(content))
	err = runHook("post", "success", args)
	assert.NoError(t, err)
	content, err = ioutil.ReadFile(tmp)
	assert.NoError(t, err)
	assert.Equal(t, "post-bar\n", string(content))
	err = runHook("pre", "no-hook", args)
	assert.NoError(t, err)
	err = runHook("pre", "failure", args)
	assert.Error(t, err)
}

func TestIsExecutable(t *testing.T) {
	assert.False(t, isExecutable("no/such/file"))
	assert.False(t, isExecutable("../../tests/fixtures/logos.zip"))
	assert.True(t, isExecutable("../../tests/hooks/pre-success"))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	config.GetConfig().Hooks = "../../tests/hooks"
	os.Exit(m.Run())
}
