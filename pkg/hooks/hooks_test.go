package hooks

import (
	"errors"
	"os"
	"path"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/stretchr/testify/assert"
)

func TestHooks(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	config.GetConfig().Hooks = "./testdata"

	t.Run("ExecuteSuccess", func(t *testing.T) {
		tmp := path.Join(t.TempDir(), "out.log")
		args := []string{tmp, "bar"}
		executed := false
		fn := func() error { executed = true; return nil }
		err := Execute("success", args, fn)
		assert.NoError(t, err)
		assert.True(t, executed)
		content, err := os.ReadFile(tmp)
		assert.NoError(t, err)
		assert.Equal(t, "post-bar\n", string(content))
	})

	t.Run("ExecutePreFails", func(t *testing.T) {
		executed := false
		fn := func() error { executed = true; return nil }
		err := Execute("failure", []string{}, fn)
		assert.Equal(t, ErrHookFailed, err)
		assert.False(t, executed)
	})

	t.Run("ExecuteFunctionFails", func(t *testing.T) {
		tmp := path.Join(t.TempDir(), "out.log")
		args := []string{tmp, "bar"}
		executed := false
		e := errors.New("fn fails")
		fn := func() error { executed = true; return e }
		err := Execute("success", args, fn)
		assert.Equal(t, e, err)
		assert.True(t, executed)
		content, err := os.ReadFile(tmp)
		assert.NoError(t, err)
		assert.Equal(t, "pre-bar\n", string(content))
	})

	t.Run("RunHooks", func(t *testing.T) {
		tmp := path.Join(t.TempDir(), "out.log")
		args := []string{tmp, "bar"}
		err := runHook("pre", "success", args)
		assert.NoError(t, err)
		content, err := os.ReadFile(tmp)
		assert.NoError(t, err)
		assert.Equal(t, "pre-bar\n", string(content))
		err = runHook("post", "success", args)
		assert.NoError(t, err)
		content, err = os.ReadFile(tmp)
		assert.NoError(t, err)
		assert.Equal(t, "post-bar\n", string(content))
		err = runHook("pre", "no-hook", args)
		assert.NoError(t, err)
		err = runHook("pre", "failure", args)
		assert.Error(t, err)
	})

	t.Run("IsExecutable", func(t *testing.T) {
		assert.False(t, isExecutable("no/such/file"))
		assert.False(t, isExecutable("../../tests/fixtures/logos.zip"))
		assert.True(t, isExecutable("./testdata/pre-success"))
	})
}
